// Package config provides configuration loading and validation for the wolf-be service.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// validEnvs is the set of accepted deployment environment names.
var validEnvs = map[string]struct{}{
	EnvDevelopment: {},
	EnvStaging:     {},
	EnvProduction:  {},
}

// validLogLevels is the set of accepted log levels.
var validLogLevels = map[string]struct{}{
	"debug": {}, "info": {}, "warn": {}, "error": {},
}

// validLogFormats is the set of accepted log output formats.
var validLogFormats = map[string]struct{}{
	"json": {}, "console": {},
}

// Validate checks all configuration fields for correctness and returns a
// combined error containing every individual validation failure. This allows
// operators to fix all misconfiguration in a single restart cycle rather than
// discovering problems one at a time.
func (c *Config) Validate() error {
	var errs []error

	errs = append(errs, validateApp(c.App)...)
	errs = append(errs, validateHTTP(c.HTTP)...)
	errs = append(errs, validateGRPC(c.GRPC, c.HTTP.Port)...)
	errs = append(errs, validateDB(c.DB)...)
	errs = append(errs, validateRedis(c.Cache.Driver, c.Redis)...)
	errs = append(errs, validateBroker(c.Broker.Driver, c.Kafka, c.RabbitMQ, c.NATS)...)
	errs = append(errs, validateLog(c.Log)...)
	errs = append(errs, validateOtel(c.Otel)...)
	errs = append(errs, validateMetrics(c.Metrics, c.HTTP.Port, c.GRPC.Port)...)
	errs = append(errs, validateCB(c.CB)...)
	errs = append(errs, validateRateLimit(c.RateLimit)...)
	errs = append(errs, validateLoadShed(c.LoadShed)...)
	errs = append(errs, validateSecurityHeaders(c.SecurityHeaders)...)
	errs = append(errs, validateOutbox(c.Outbox)...)
	errs = append(errs, validateJWT(c.JWT)...)
	errs = append(errs, validateBcrypt(c.Bcrypt)...)
	errs = append(errs, validateSession(c.Session)...)

	if c.App.Env == EnvProduction {
		errs = append(errs, validateProductionSafety(c)...)
	}

	return errors.Join(errs...)
}

// knownWeakSecrets is a blocklist of default/example JWT secrets that must never
// be used in production. These are commonly found in example configs and tutorials.
var knownWeakSecrets = map[string]struct{}{
	"change-me-in-production":   {},
	"your-secret-key":           {},
	"secret":                    {},
	"supersecret":               {},
	"my-secret-key":             {},
	"jwt-secret":                {},
	"development-secret-key":    {},
	"wolf-dev-secret":           {},
	"wolf-prime-dev-secret-key": {},
}

// validateProductionSafety enforces fail-fast rules that prevent deploying
// insecure or development configurations to production environments.
func validateProductionSafety(c *Config) []error {
	var errs []error

	if c.Broker.Driver == BrokerInProcess {
		errs = append(errs, fmt.Errorf("production: broker.driver must not be %q (use nats)", BrokerInProcess))
	}

	if c.Cache.Driver == "noop" || c.Cache.Driver == "" {
		errs = append(errs, fmt.Errorf("production: cache.driver must not be empty or %q (token revocation requires redis)", "noop"))
	}

	if c.Otel.Insecure {
		errs = append(errs, fmt.Errorf("production: otel.insecure must be false (use TLS for trace export)"))
	}

	secret := strings.ToLower(strings.TrimSpace(c.JWT.SecretKey))
	if _, weak := knownWeakSecrets[secret]; weak {
		errs = append(errs, fmt.Errorf("production: jwt.secret_key is a known weak default — use a strong random secret"))
	}

	if c.RateLimit.RPS > 0 && len(c.HTTP.TrustedProxies) == 0 {
		errs = append(errs, fmt.Errorf("production: http.trusted_proxies must be set when rate limiting is enabled (prevents rate-limit by proxy IP)"))
	}

	return errs
}

// defaultK8sGracePeriod is the standard Kubernetes terminationGracePeriodSeconds.
// shutdown_timeout must be strictly less than this to avoid SIGKILL during drain.
const defaultK8sGracePeriod = 30 * time.Second

// validateApp checks application-level configuration.
func validateApp(cfg AppConfig) []error {
	var errs []error

	if cfg.Name == "" {
		errs = append(errs, fmt.Errorf("app.name must not be empty"))
	}

	if _, ok := validEnvs[cfg.Env]; !ok {
		errs = append(errs, fmt.Errorf("app.env %q is invalid: must be one of development, staging, production", cfg.Env))
	}

	if cfg.Debug && cfg.Env == EnvProduction {
		errs = append(errs, fmt.Errorf("app.debug must be false in production (exposes pprof and verbose output)"))
	}

	if cfg.ShutdownTimeout >= defaultK8sGracePeriod {
		errs = append(errs, fmt.Errorf(
			"app.shutdown_timeout (%s) must be less than K8s terminationGracePeriodSeconds (%s) to avoid SIGKILL during drain",
			cfg.ShutdownTimeout, defaultK8sGracePeriod,
		))
	}

	return errs
}

// validateHTTP checks HTTP server configuration.
func validateHTTP(cfg HTTPConfig) []error {
	var errs []error

	if err := validatePort("http.port", cfg.Port); err != nil {
		errs = append(errs, err)
	}

	if cfg.ReadTimeout <= 0 {
		errs = append(errs, fmt.Errorf("http.read_timeout must be positive"))
	}

	if cfg.WriteTimeout <= 0 {
		errs = append(errs, fmt.Errorf("http.write_timeout must be positive"))
	}

	if cfg.IdleTimeout <= 0 {
		errs = append(errs, fmt.Errorf("http.idle_timeout must be positive"))
	}

	if cfg.MaxHeaderBytes <= 0 {
		errs = append(errs, fmt.Errorf("http.max_header_bytes must be positive"))
	}

	errs = append(errs, validateCORS(cfg.CORS)...)

	return errs
}

// validateCORS checks CORS configuration for unsafe combinations.
func validateCORS(cfg CORSConfig) []error {
	var errs []error

	if cfg.AllowCredentials {
		for _, origin := range cfg.AllowedOrigins {
			if origin == "*" {
				errs = append(errs, fmt.Errorf("http.cors: allow_credentials=true is incompatible with wildcard origin \"*\""))
				break
			}
		}
	}

	if cfg.MaxAge < 0 {
		errs = append(errs, fmt.Errorf("http.cors.max_age must not be negative"))
	}

	return errs
}

// validateGRPC checks gRPC server configuration and ensures its port does not
// collide with the HTTP port.
func validateGRPC(cfg GRPCConfig, httpPort int) []error {
	var errs []error

	if err := validatePort("grpc.port", cfg.Port); err != nil {
		errs = append(errs, err)
	}

	if cfg.Port != 0 && cfg.Port == httpPort {
		errs = append(errs, fmt.Errorf("grpc.port (%d) must differ from http.port (%d)", cfg.Port, httpPort))
	}

	if cfg.MaxRecvMsgSize <= 0 {
		errs = append(errs, fmt.Errorf("grpc.max_recv_msg_size must be positive"))
	}

	if cfg.MaxSendMsgSize <= 0 {
		errs = append(errs, fmt.Errorf("grpc.max_send_msg_size must be positive"))
	}

	if cfg.KeepaliveTime <= 0 {
		errs = append(errs, fmt.Errorf("grpc.keepalive_time must be positive"))
	}

	if cfg.KeepaliveTimeout <= 0 {
		errs = append(errs, fmt.Errorf("grpc.keepalive_timeout must be positive"))
	}

	return errs
}

// validateDB checks both write and read pool configurations.
func validateDB(cfg DBConfig) []error {
	var errs []error

	errs = append(errs, validatePool("db.write", cfg.Write)...)
	errs = append(errs, validatePool("db.read", cfg.Read)...)

	return errs
}

// validatePool checks a single database connection pool configuration.
func validatePool(prefix string, cfg PoolConfig) []error {
	var errs []error

	if cfg.DSN == "" {
		errs = append(errs, fmt.Errorf("%s.dsn must not be empty", prefix))
	}

	if cfg.MaxOpenConns <= 0 {
		errs = append(errs, fmt.Errorf("%s.max_open_conns must be positive", prefix))
	}

	if cfg.MaxIdleConns <= 0 {
		errs = append(errs, fmt.Errorf("%s.max_idle_conns must be positive", prefix))
	}

	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		errs = append(errs, fmt.Errorf("%s.max_idle_conns (%d) must not exceed max_open_conns (%d)",
			prefix, cfg.MaxIdleConns, cfg.MaxOpenConns))
	}

	if cfg.ConnMaxLifetime <= 0 {
		errs = append(errs, fmt.Errorf("%s.conn_max_lifetime must be positive", prefix))
	}

	if cfg.ConnMaxIdleTime <= 0 {
		errs = append(errs, fmt.Errorf("%s.conn_max_idle_time must be positive", prefix))
	}

	return errs
}

// validateRedis checks Redis configuration when the cache driver is "redis"
// and rejects unknown driver values.
func validateRedis(driver string, cfg RedisConfig) []error {
	var errs []error

	switch driver {
	case CacheDriverRedis: // validated below
	case CacheDriverNoop:
		return nil
	case "":
		return []error{fmt.Errorf("cache.driver must not be empty")}
	default:
		return []error{fmt.Errorf("cache.driver %q is invalid: must be redis or noop", driver)}
	}

	if cfg.Addr == "" {
		errs = append(errs, fmt.Errorf("redis.addr must not be empty when cache driver is redis"))
	}

	if cfg.PoolSize <= 0 {
		errs = append(errs, fmt.Errorf("redis.pool_size must be positive"))
	}

	if cfg.DB < 0 {
		errs = append(errs, fmt.Errorf("redis.db must be >= 0"))
	}

	if cfg.DialTimeout <= 0 {
		errs = append(errs, fmt.Errorf("redis.dial_timeout must be positive"))
	}

	if cfg.ReadTimeout <= 0 {
		errs = append(errs, fmt.Errorf("redis.read_timeout must be positive"))
	}

	if cfg.WriteTimeout <= 0 {
		errs = append(errs, fmt.Errorf("redis.write_timeout must be positive"))
	}

	return errs
}

// validateBroker checks message broker configuration.
func validateBroker(driver string, kafka KafkaConfig, rabbitmq RabbitMQConfig, nats NATSConfig) []error {
	var errs []error

	if driver == "" {
		errs = append(errs, fmt.Errorf("broker.driver must not be empty"))
	}

	switch driver {
	case BrokerKafka:
		errs = append(errs, fmt.Errorf("broker.driver %q is not yet implemented (use inprocess or nats)", driver))
	case BrokerRabbitMQ:
		errs = append(errs, fmt.Errorf("broker.driver %q is not yet implemented (use inprocess or nats)", driver))
	case BrokerNATS:
		if nats.URL == "" {
			errs = append(errs, fmt.Errorf("nats.url must not be empty"))
		}
		if nats.Stream == "" {
			errs = append(errs, fmt.Errorf("nats.stream must not be empty"))
		}
		if len(nats.Subjects) == 0 {
			errs = append(errs, fmt.Errorf("nats.subjects must have at least one entry"))
		}
	}

	return errs
}

// validateLog checks logging configuration.
func validateLog(cfg LoggingConfig) []error {
	var errs []error

	if _, ok := validLogLevels[cfg.Level]; !ok {
		errs = append(errs, fmt.Errorf("log.level %q is invalid: must be debug, info, warn, or error", cfg.Level))
	}

	if _, ok := validLogFormats[cfg.Format]; !ok {
		errs = append(errs, fmt.Errorf("log.format %q is invalid: must be json or console", cfg.Format))
	}

	return errs
}

// validateOtel checks distributed tracing configuration.
func validateOtel(cfg TracingConfig) []error {
	if !cfg.Enabled {
		return nil
	}

	var errs []error

	if cfg.Exporter == "" {
		errs = append(errs, fmt.Errorf("otel.exporter must not be empty when tracing is enabled"))
	}
	if cfg.Endpoint == "" {
		errs = append(errs, fmt.Errorf("otel.endpoint must not be empty when tracing is enabled"))
	}
	if cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		errs = append(errs, fmt.Errorf("otel.sample_rate must be between 0.0 and 1.0"))
	}

	return errs
}

// validateMetrics checks Prometheus metrics configuration and guards against
// port collisions with the HTTP and gRPC ports.
func validateMetrics(cfg MetricsConfig, httpPort, grpcPort int) []error {
	if !cfg.Enabled {
		return nil
	}

	var errs []error

	if err := validatePort("metrics.port", cfg.Port); err != nil {
		errs = append(errs, err)
	}
	if cfg.Port == httpPort {
		errs = append(errs, fmt.Errorf("metrics.port (%d) must differ from http.port (%d)", cfg.Port, httpPort))
	}
	if cfg.Port == grpcPort {
		errs = append(errs, fmt.Errorf("metrics.port (%d) must differ from grpc.port (%d)", cfg.Port, grpcPort))
	}

	return errs
}

// validateCB checks circuit breaker configuration.
func validateCB(cfg CircuitBreakerConfig) []error {
	var errs []error

	if cfg.MaxRequests <= 0 {
		errs = append(errs, fmt.Errorf("cb.max_requests must be positive"))
	}

	if cfg.Interval <= 0 {
		errs = append(errs, fmt.Errorf("cb.interval must be positive"))
	}

	if cfg.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("cb.timeout must be positive"))
	}

	return errs
}

// validateRateLimit checks rate limiter configuration.
// RPS=0 explicitly disables the per-IP rate limiter (useful during incidents).
func validateRateLimit(cfg RateLimitConfig) []error {
	if cfg.RPS == 0 {
		return nil // disabled
	}

	var errs []error

	if cfg.RPS < 0 {
		errs = append(errs, fmt.Errorf("rate_limit.rps must be >= 0 (0 = disabled)"))
	}

	if cfg.Burst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.burst must be positive when rate limiting is enabled"))
	}

	return errs
}

// validateLoadShed checks load shedding configuration.
func validateLoadShed(cfg LoadShedConfig) []error {
	if cfg.MaxConcurrent < 0 {
		return []error{fmt.Errorf("load_shed.max_concurrent must not be negative")}
	}
	return nil
}

// validateSecurityHeaders checks security headers configuration.
func validateSecurityHeaders(cfg SecurityHeadersConfig) []error {
	if !cfg.Enabled {
		return nil
	}
	var errs []error
	if cfg.HSTS && cfg.HSTSMaxAge < 0 {
		errs = append(errs, fmt.Errorf("security_headers.hsts_max_age must not be negative"))
	}
	return errs
}

// validateOutbox checks transactional outbox relay configuration.
func validateOutbox(cfg OutboxConfig) []error {
	var errs []error

	if cfg.PollInterval <= 0 {
		errs = append(errs, fmt.Errorf("outbox.poll_interval must be positive"))
	}

	if cfg.BatchSize <= 0 {
		errs = append(errs, fmt.Errorf("outbox.batch_size must be positive"))
	}

	if cfg.MaxRetries <= 0 {
		errs = append(errs, fmt.Errorf("outbox.max_retries must be positive"))
	}

	if cfg.Retention <= 0 {
		errs = append(errs, fmt.Errorf("outbox.retention must be positive"))
	}

	if cfg.PollTimeout < 0 {
		errs = append(errs, fmt.Errorf("outbox.poll_timeout must not be negative"))
	}

	if cfg.HealthThreshold < 0 {
		errs = append(errs, fmt.Errorf("outbox.health_threshold must not be negative"))
	}

	return errs
}

// validateJWT checks JWT signing key material and token TTL settings.
func validateJWT(cfg JWTConfig) []error {
	var errs []error

	if strings.Contains(cfg.SecretKey, "${") {
		errs = append(errs, fmt.Errorf("jwt.secret_key contains unexpanded variable placeholder"))
	}

	switch cfg.SigningMethod {
	case "HS256":
		if len(cfg.SecretKey) < 32 {
			errs = append(errs, fmt.Errorf("jwt.secret_key must be at least 32 characters for HS256"))
		}
	case "RS256":
		if cfg.PrivateKeyPath == "" {
			errs = append(errs, fmt.Errorf("jwt.private_key_path must not be empty for RS256"))
		}
		if cfg.PublicKeyPath == "" {
			errs = append(errs, fmt.Errorf("jwt.public_key_path must not be empty for RS256"))
		}
	default:
		errs = append(errs, fmt.Errorf("jwt.signing_method %q is invalid: must be HS256 or RS256", cfg.SigningMethod))
	}

	if cfg.AccessTokenTTL < time.Minute {
		errs = append(errs, fmt.Errorf("jwt.access_token_ttl must be at least 1m"))
	}
	if cfg.AccessTokenTTL > time.Hour {
		errs = append(errs, fmt.Errorf("jwt.access_token_ttl must be at most 1h"))
	}

	const maxRefreshTTL = 2160 * time.Hour // 90 days

	if cfg.RefreshTokenTTL < time.Hour {
		errs = append(errs, fmt.Errorf("jwt.refresh_token_ttl must be at least 1h"))
	}
	if cfg.RefreshTokenTTL > maxRefreshTTL {
		errs = append(errs, fmt.Errorf("jwt.refresh_token_ttl must be at most 2160h (90 days)"))
	}

	if cfg.Issuer == "" {
		errs = append(errs, fmt.Errorf("jwt.issuer must not be empty"))
	}

	if len(cfg.Audience) != 1 {
		errs = append(errs, fmt.Errorf("jwt.audience must contain exactly one entry, got %d", len(cfg.Audience)))
	}

	return errs
}

// validateBcrypt checks bcrypt hashing parameters.
func validateBcrypt(cfg BcryptConfig) []error {
	var errs []error

	if cfg.Cost < 10 {
		errs = append(errs, fmt.Errorf("bcrypt.cost must be at least 10"))
	}

	if cfg.Cost > 14 {
		errs = append(errs, fmt.Errorf("bcrypt.cost must be at most 14"))
	}

	return errs
}

// validateSession checks session management configuration.
func validateSession(_ SessionConfig) []error {
	return nil
}

// validatePort returns an error if the given TCP port number is not in the
// valid range 1–65535.
func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s (%d) must be between 1 and 65535", name, port)
	}

	return nil
}
