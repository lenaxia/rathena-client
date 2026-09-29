// Fuzz targets for the session framing engine and the generated receive
// dispatch table.
//
// Invariants under test:
//
//  1. Feed() must never panic — regardless of byte content, framing faults
//     must surface as returned errors (ErrUnknownPacket) or UnknownPacketEvent
//     callbacks, never as panics.
//  2. Registered decode functions must never panic on frames the framer has
//     length-validated: fixed-length packets are handed over at their declared
//     table length; variable-length packets carry their own embedded length.
//     A panic here is a real decoder bug (missing bounds check), not noise.
//
// Running:
//
//	go test ./pkg/session/ -run Fuzz -fuzz FuzzFeedMapSessionWithDispatch -fuzztime 30s
//
// The seed corpus is loaded from the replay fixtures in testdata/, so every
// run starts from real captured traffic. New corpus entries discovered by the
// fuzzer land in testdata/fuzz/ and should be committed.
package session

import (
	"testing"
)

// fuzzDispatchAll registers every receiveDispatch entry directly against the
// session's handler table, bypassing the typed RegisterSemanticHandler layer.
// This drives the raw decode functions with frames exactly as Feed() would
// deliver them in production.
//
// IDs frozen in knownLengthOverread (dispatch_length_audit_test.go) are
// skipped until the codegen packetver-evaluation fix lands — their crashes
// are already enumerated and gated by TestReceiveDispatchLengthAudit.
func fuzzDispatchAll(s *MapSession) {
	for _, entries := range receiveDispatch {
		for _, e := range entries {
			entry := e
			if _, broken := knownLengthOverread[entry.id]; broken {
				continue
			}
			s.registerHandler(entry.id, func(data []byte, pv uint32) {
				_ = entry.fn(data, pv)
			})
		}
	}
}

// newFuzzMapSession builds a MapSession with the full decode dispatch table
// and a non-nil unknown-packet callback, maximizing covered code per byte fed.
func newFuzzMapSession(t *testing.T, pv uint32) *MapSession {
	t.Helper()
	s := NewMapSession(pv)
	fuzzDispatchAll(s)
	s.SetUnknownPacketHandler(func(_ UnknownPacketEvent) {})
	return s
}

// addFixtureSeeds seeds a fuzz target with the S→C phases of every replay
// fixture in testdata/. Skips silently when a fixture is absent so the fuzz
// targets still work from a fresh checkout with -run.
func addFixtureSeeds(f *testing.F, names ...string) {
	for _, name := range names {
		fix, err := loadFixture("testdata/" + name)
		if err != nil {
			continue
		}
		if len(fix.login) > 0 {
			f.Add(fix.login)
		}
		if len(fix.char) > 0 {
			f.Add(fix.char)
		}
		if len(fix.mapPhase) > 0 {
			f.Add(fix.mapPhase)
		}
	}
}

// FuzzFeedMapSessionWithDispatch is the primary target: random bytes through
// the map framing engine with every generated decoder armed.
func FuzzFeedMapSessionWithDispatch(f *testing.F) {
	addFixtureSeeds(f, "auth_20200401.fixture", "movement_20200401.fixture")
	f.Fuzz(func(t *testing.T, data []byte) {
		s := newFuzzMapSession(t, 20200401)
		// Chunked feeds exercise the incomplete-frame resume path in addition
		// to whole-buffer delivery.
		for i := 0; i < len(data); i += 7 {
			end := i + 7
			if end > len(data) {
				end = len(data)
			}
			if err := s.Feed(data[i:end]); err != nil {
				// Faulted stream: a returned error is acceptable; a panic is not.
				return
			}
		}
	})
}

// FuzzFeedLoginSession fuzzes the login framing engine in isolation.
func FuzzFeedLoginSession(f *testing.F) {
	addFixtureSeeds(f, "auth_20200401.fixture")
	f.Fuzz(func(t *testing.T, data []byte) {
		s := NewLoginSession(20200401)
		s.SetUnknownPacketHandler(func(_ UnknownPacketEvent) {})
		_ = s.Feed(data)
	})
}

// FuzzFeedCharSession fuzzes the char framing engine in isolation.
func FuzzFeedCharSession(f *testing.F) {
	addFixtureSeeds(f, "auth_20200401.fixture")
	f.Fuzz(func(t *testing.T, data []byte) {
		s := NewCharSession(20200401)
		s.SetUnknownPacketHandler(func(_ UnknownPacketEvent) {})
		_ = s.Feed(data)
	})
}
