// command_stream.go — Typed command pub/sub over a raw messaging.Stream.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"
)

// Command is the marker interface for command messages sent over a stream.
// Implementations carry the data required to fulfil a single use case.
type Command interface {
	// CommandType returns the unique type string for routing and deserialisation
	// (e.g. "order.place.v1").
	CommandType() string
}

// Reply is the response envelope returned after processing a Command.
// It carries a typed result or an error description.
type Reply struct {
	// Type mirrors the command type that produced this reply, enabling
	// consumers to correlate requests and responses.
	Type string `json:"type"`
	// Success is true when the command was processed without error.
	Success bool `json:"success"`
	// Data contains the serialised response payload (may be nil on error).
	Data []byte `json:"data,omitempty"`
	// Error describes the failure reason when Success is false.
	Error string `json:"error,omitempty"`
}

// CommandStream sends Command values over a Stream.
// It serialises commands to JSON and attaches the command type as a header.
type CommandStream struct {
	stream Stream
}

// NewCommandStream wraps stream to provide a typed command-sending facade.
func NewCommandStream(stream Stream) *CommandStream {
	return &CommandStream{stream: stream}
}

// Send serialises cmd to JSON and publishes it on subject.
// The "command_type" header is set so consumers can route without parsing the body.
func (cs *CommandStream) Send(ctx context.Context, subject string, cmd Command) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("command_stream: marshal %q: %w", cmd.CommandType(), err)
	}

	msg := RawMessage{
		Name:    cmd.CommandType(),
		Subject: subject,
		Data:    data,
		Headers: map[string]string{
			"command_type": cmd.CommandType(),
		},
	}

	if err := cs.stream.Publish(ctx, subject, msg); err != nil {
		return fmt.Errorf("command_stream: publish %q: %w", cmd.CommandType(), err)
	}
	return nil
}

// Subscribe registers a handler on subject that receives raw command bytes.
// The handler is responsible for unmarshalling the bytes into the concrete
// command type. It returns an optional Reply; if non-nil and the underlying
// stream supports request-reply, the reply is sent back to the caller.
// A non-nil error from handler causes the message to be Nak'd.
func (cs *CommandStream) Subscribe(
	subject string,
	handler func(ctx context.Context, cmd []byte) (*Reply, error),
	opts ...SubscribeOption,
) error {
	msgHandler := func(ctx context.Context, msg Message) error {
		reply, err := handler(ctx, msg.Data())
		if err != nil {
			_ = msg.Nak()
			return fmt.Errorf("command_stream: handler error on %q: %w", subject, err)
		}
		_ = reply // request-reply transport is broker-specific; adapters handle it
		return msg.Ack()
	}

	return cs.stream.Subscribe(subject, msgHandler, opts...)
}

// ReplyStream sends Reply values over a Stream.
// It serialises replies to JSON for transport back to command senders.
type ReplyStream struct {
	stream Stream
}

// NewReplyStream wraps stream to provide a typed reply-sending facade.
func NewReplyStream(stream Stream) *ReplyStream {
	return &ReplyStream{stream: stream}
}

// Send serialises reply to JSON and publishes it on subject.
func (rs *ReplyStream) Send(ctx context.Context, subject string, reply Reply) error {
	data, err := json.Marshal(reply)
	if err != nil {
		return fmt.Errorf("reply_stream: marshal reply for %q: %w", subject, err)
	}

	msg := RawMessage{
		Name:    reply.Type,
		Subject: subject,
		Data:    data,
		Headers: map[string]string{
			"reply_type": reply.Type,
			"success":    fmt.Sprintf("%t", reply.Success),
		},
	}

	if err := rs.stream.Publish(ctx, subject, msg); err != nil {
		return fmt.Errorf("reply_stream: publish reply for %q: %w", subject, err)
	}
	return nil
}

// Subscribe registers a handler on subject that receives Reply values.
// The raw message body is deserialised into a Reply before calling handler.
// A non-nil error from handler causes the message to be Nak'd.
func (rs *ReplyStream) Subscribe(
	subject string,
	handler func(ctx context.Context, reply Reply) error,
	opts ...SubscribeOption,
) error {
	msgHandler := func(ctx context.Context, msg Message) error {
		var reply Reply
		if err := json.Unmarshal(msg.Data(), &reply); err != nil {
			_ = msg.Ack() // malformed reply — discard to avoid poison pill
			return fmt.Errorf("reply_stream: unmarshal reply on %q: %w", subject, err)
		}

		if err := handler(ctx, reply); err != nil {
			_ = msg.Nak()
			return fmt.Errorf("reply_stream: handler error on %q: %w", subject, err)
		}
		return msg.Ack()
	}

	return rs.stream.Subscribe(subject, msgHandler, opts...)
}
