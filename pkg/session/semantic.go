package session

import (
	"errors"
	"fmt"
	"io"
)

// ErrWrongSendType is returned by Send when req is not the expected send.* type
// for the given action. Exported so callers can use errors.Is for precise handling.
var ErrWrongSendType = errors.New("session: Send called with wrong request type for action")

// SendEncoderFunc is the type stored in the send dispatch registry.
// It accepts the request as interface{} (the concrete send.* struct) and the
// current packetver, and returns the fully-encoded byte slice ready to send.
// The encode function has already written the correct wire packet ID (including
// any packetver-dependent shuffle) into bytes 0–1; Send applies only XOR
// obfuscation on top. Returns ErrWrongSendType if req is not the expected type.
type SendEncoderFunc func(req interface{}, pv uint32) ([]byte, error)

// sendRegistry holds the registered encoder for each send-direction SemanticAction.
// Populated during package init by pkg/encode/register.go.
// Array-indexed by SemanticAction value for O(1) lookup with zero allocation.
var sendRegistry [maxSemanticAction + 1]SendEncoderFunc

// RegisterSendEncoder registers fn as the encoder for action.
// Must be called from init() only. Not goroutine-safe after program init.
// Panics if called twice for the same action — indicates a codegen bug producing
// duplicate init() registrations.
func RegisterSendEncoder(action SemanticAction, fn SendEncoderFunc) {
	if sendRegistry[action] != nil {
		panic(fmt.Sprintf("session: RegisterSendEncoder called twice for action %v", action))
	}
	sendRegistry[action] = fn
}

// RegisterSemanticHandler registers fn to be called whenever the session receives
// any packet that maps to action. fn must be a func(E) where E is the events.*
// struct corresponding to action.
//
// E must be the concrete struct value type (e.g. events.ActorMoved), NOT a
// pointer (e.g. *events.ActorMoved). All decode functions return struct values,
// not pointers. Passing a pointer-receiver func will compile but panic at the
// first packet dispatch with a type mismatch message.
//
// IMPORTANT: string/[]byte fields in the decoded event (e.g. event.Name) are
// zero-copy aliases into the session receive buffer. They are valid only for the
// duration of the handler callback. Do NOT store them past the handler's return.
// To retain a string, copy it first: name = decode.CopyString(event.Name).
//
// All packetver variants of the action are registered simultaneously. If the
// session's packetver means only one variant will ever appear on the wire, the
// others will simply never fire — this is harmless.
//
// If action has no receive-direction dispatch entries (unknown or send-only action),
// RegisterSemanticHandler panics immediately.
//
// If a packet arrives and the decoded event cannot be type-asserted to E, the
// handler panics at dispatch time. This makes misconfiguration fail at the first
// received packet rather than silently dropping events.
//
// A second call to RegisterSemanticHandler for the same action silently overwrites
// the first registration for all covered packet IDs. This matches the underlying
// RegisterHandler contract.
//
// This is a free function rather than a method on MapSession because Go does not
// support generic methods.
func RegisterSemanticHandler[E any](s *MapSession, action SemanticAction, fn func(E)) {
	entries, ok := receiveDispatch[action]
	if !ok {
		panic(fmt.Sprintf("session: RegisterSemanticHandler: unknown or send-only action %v", action))
	}
	for _, e := range entries {
		e := e // capture loop variable
		s.registerHandler(e.id, func(data []byte, pv uint32) {
			raw := e.fn(data, pv)
			typed, ok := raw.(E)
			if !ok {
				panic(fmt.Sprintf("session: handler type mismatch for action %v packet 0x%04X: "+
					"got %T, handler expects %T", action, e.id, raw, *new(E)))
			}
			fn(typed)
		})
	}
}

// Send encodes req using the registered encode function for action, applies XOR
// packet ID obfuscation if enabled, and writes the result to w.
//
// req must be the send.* struct value corresponding to action. If the concrete
// type does not match what was registered, Send returns ErrWrongSendType.
// Send accepts req as interface{} because the type check is performed inside
// the registered SendEncoderFunc closure at runtime — Go generics cannot provide
// compile-time safety here since the registry maps SemanticAction to interface{}.
// Callers should treat this as a typed call: always pass the exact send.* struct
// type documented for the action.
//
// The encode function (registered by pkg/encode/register.go at init time) is
// responsible for writing the correct wire packet ID — including any
// packetver-dependent shuffle — into bytes 0–1 of the buffer. Send reads those
// bytes back, applies s.encodePacketID (rolling-key XOR obfuscation, a no-op when
// obfuscation is not active), and writes the final buffer to w.
//
// Send does NOT call ShuffledCtoSID — shuffle is the encode function's
// responsibility. Send only applies XOR obfuscation.
func Send(s *MapSession, w io.Writer, action SemanticAction, req interface{}) error {
	if int(action) >= len(sendRegistry) || sendRegistry[action] == nil {
		return fmt.Errorf("session: Send: unknown or receive-only action %v", action)
	}
	fn := sendRegistry[action]
	data, err := fn(req, s.core.packetver)
	if err != nil {
		return err
	}
	if len(data) >= 2 {
		id := uint16(data[0]) | uint16(data[1])<<8
		s.encodePacketID(&id)
		data[0] = byte(id)
		data[1] = byte(id >> 8)
	}
	_, err = w.Write(data)
	return err
}
