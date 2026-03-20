// Package grpc provides the production gRPC server for the wolf-be service.
// It wraps google.golang.org/grpc with keepalive settings, configurable message
// size limits, and a chained unary interceptor stack.
package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/config"
)

// Server wraps a *grpclib.Server with the port and logger needed to drive its
// lifecycle (Start / Stop).
type Server struct {
	server   *grpclib.Server
	listener net.Listener
	port     int
	logger   *zap.Logger
	noop     bool
}

// NewNoop creates a disabled Server that accepts RegisterGRPC calls but never listens.
// Used when grpc.enabled=false. Start/Stop are safe no-ops.
func NewNoop() *Server {
	srv := grpclib.NewServer()
	return &Server{server: srv, logger: zap.NewNop(), noop: true}
}

// New creates a *Server configured from cfg with the given unary interceptors
// chained in order. The gRPC server reflection service is registered
// automatically so that tools like grpcurl can introspect the server.
//
// interceptors are applied in the order provided (first = outermost).
func New(cfg config.GRPCConfig, logger *zap.Logger, interceptors ...grpclib.UnaryServerInterceptor) *Server {
	opts := []grpclib.ServerOption{
		grpclib.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
		grpclib.MaxSendMsgSize(cfg.MaxSendMsgSize),
		grpclib.KeepaliveParams(keepalive.ServerParameters{
			Time:    cfg.KeepaliveTime,
			Timeout: cfg.KeepaliveTimeout,
		}),
	}

	if len(interceptors) > 0 {
		opts = append(opts, grpclib.ChainUnaryInterceptor(interceptors...))
	}

	srv := grpclib.NewServer(opts...)
	reflection.Register(srv)

	return &Server{
		server: srv,
		port:   cfg.Port,
		logger: logger,
	}
}

// GRPCServer returns the underlying *grpclib.Server so that generated
// service registration functions (pb.RegisterXxxServer) can be called by
// module wire sets.
func (s *Server) GRPCServer() *grpclib.Server {
	return s.server
}

// Start opens a TCP listener on the configured port and begins serving gRPC
// requests. It blocks until the server is stopped. Callers should run Start
// in a separate goroutine. The listener is stored on the Server struct so that
// Stop can close it if Serve fails to take ownership.
func (s *Server) Start() error {
	if s.noop {
		return nil
	}
	addr := fmt.Sprintf(":%d", s.port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc server: listen on %s: %w", addr, err)
	}
	s.listener = lis

	s.logger.Info("gRPC server starting", zap.String("addr", addr))
	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("grpc server: serve: %w", err)
	}
	return nil
}

// Stop initiates a graceful shutdown of the gRPC server. It gives in-flight
// RPCs until ctx is done to complete, after which the server is force-stopped.
// The stored listener is closed to ensure no file descriptor leak.
func (s *Server) Stop(ctx context.Context) error {
	if s.noop {
		return nil
	}
	s.logger.Info("gRPC server stopping")

	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	var stopErr error
	select {
	case <-done:
	case <-ctx.Done():
		s.server.Stop()
		// Wait for the goroutine to exit so we don't leak it. Stop() is
		// synchronous so GracefulStop should return almost immediately.
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
		}
		stopErr = fmt.Errorf("grpc server: graceful stop timed out: %w", ctx.Err())
	}

	// Ensure the listener is closed even if Serve never took ownership.
	if s.listener != nil {
		_ = s.listener.Close()
	}

	return stopErr
}
