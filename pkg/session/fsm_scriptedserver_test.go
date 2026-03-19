// scriptedserver_test.go provides ScriptedServer, a fixture-based server that
// replays captured S→C bytes over net.Pipe connections for replay tests.
//
// The server reads a .fixture file and serves three phase connections in order
// (login, char, map). It tolerates interleaved C→S bytes from the FSM by
// consuming them asynchronously while writing S→C bytes.
package session

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net"
	"os"
	"time"
)

// fixtureData holds the parsed S→C bytes for all three phases.
type fixtureData struct {
	packetver uint32
	login     []byte
	char      []byte
	mapPhase  []byte
}

// loadFixture reads a .fixture file written by cmd/gen-fixture.
// Format: "RATF" + uint32 version=1 + uint32 packetver +
// [phase-tag(1) + len(4) + data(len)] × 3 + "END "
func loadFixture(path string) (*fixtureData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loadFixture: %w", err)
	}
	if len(b) < 16 {
		return nil, fmt.Errorf("loadFixture: too short (%d bytes)", len(b))
	}
	if string(b[0:4]) != "RATF" {
		return nil, fmt.Errorf("loadFixture: bad magic %q", b[0:4])
	}
	version := binary.LittleEndian.Uint32(b[4:8])
	if version != 1 {
		return nil, fmt.Errorf("loadFixture: unsupported version %d", version)
	}
	packetver := binary.LittleEndian.Uint32(b[8:12])

	pos := 12
	phases := make([][]byte, 3)
	expectedTags := []byte{0x01, 0x02, 0x03}
	for i, expectedTag := range expectedTags {
		if pos+5 > len(b) {
			return nil, fmt.Errorf("loadFixture: truncated at phase %d", i)
		}
		tag := b[pos]
		if tag != expectedTag {
			return nil, fmt.Errorf("loadFixture: phase %d: expected tag 0x%02x, got 0x%02x", i, expectedTag, tag)
		}
		n := int(binary.LittleEndian.Uint32(b[pos+1 : pos+5]))
		pos += 5
		if pos+n > len(b) {
			return nil, fmt.Errorf("loadFixture: phase %d: data truncated (need %d, have %d)", i, n, len(b)-pos)
		}
		phases[i] = make([]byte, n)
		copy(phases[i], b[pos:pos+n])
		pos += n
	}

	if pos+4 > len(b) || string(b[pos:pos+4]) != "END " {
		return nil, fmt.Errorf("loadFixture: missing END magic")
	}

	return &fixtureData{
		packetver: packetver,
		login:     phases[0],
		char:      phases[1],
		mapPhase:  phases[2],
	}, nil
}

// ScriptedServer replays fixture phases over net.Pipe connections.
// It provides a Dialer for use with ConnectionFSM.
//
// The server drains all C→S bytes concurrently while writing S→C bytes,
// tolerating any interleaving of C→S traffic.
type ScriptedServer struct {
	fixture   *fixtureData
	rng       *rand.Rand
	dialCount int
}

// newScriptedServer creates a ScriptedServer. seed determines chunk splitting.
// Use a hash of the test name for determinism:
//
//	h := fnv.New64a(); h.Write([]byte(t.Name())); seed := int64(h.Sum64())
func newScriptedServer(fixture *fixtureData, seed int64) *ScriptedServer {
	return &ScriptedServer{
		fixture: fixture,
		rng:     rand.New(rand.NewSource(seed)),
	}
}

// seedFromName produces a deterministic seed from a string.
func seedFromName(name string) int64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return int64(h.Sum64())
}

// Dialer returns a Dialer suitable for use with ConnectionFSM.
// The first call serves the login phase, second the char phase, third the map phase.
func (s *ScriptedServer) Dialer() Dialer {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		i := s.dialCount
		s.dialCount++

		var phaseData []byte
		switch i {
		case 0: // login
			phaseData = s.fixture.login
		case 1: // char
			phaseData = s.fixture.char
		case 2: // map
			phaseData = s.fixture.mapPhase
		default:
			return nil, fmt.Errorf("ScriptedServer: unexpected dial #%d", i)
		}

		client, server := net.Pipe()
		go s.servePhase(server, phaseData, i)
		return client, nil
	}
}

// initialCSSize is the size of the first C→S packet the FSM sends on each phase
// connection. The server must read this before writing anything, to unblock
// the FSM's initial write on the synchronous net.Pipe.
//
// Phase 0 (login): CA_LOGIN = 55 bytes (packetver 20200401, 0x0064 variant)
// Phase 1 (char):  CH_ENTER = 17 bytes (0x0065)
// Phase 2 (map):   CZ_ENTER = 19 bytes (0x0436)
var initialCSSize = [3]int{55, 17, 19}

// servePhase writes phaseData to conn in random-sized chunks while concurrently
// draining any C→S bytes the FSM sends.
//
// Protocol for each phase:
//  1. Drain the initial C→S packet (synchronous, to unblock the FSM write)
//  2. For char phase: write the 4-byte account ID echo (rAthena char_clif.cpp:851-853)
//  3. Start background drain goroutine for remaining C→S traffic
//  4. Write S→C fixture bytes in random-sized chunks
func (s *ScriptedServer) servePhase(conn net.Conn, phaseData []byte, phaseIdx int) {
	defer conn.Close()

	// Step 1: Drain the initial C→S packet so the FSM's first Write doesn't deadlock.
	initialBuf := make([]byte, initialCSSize[phaseIdx])
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(conn, initialBuf); err != nil {
		return
	}

	// Step 2: For the char phase, write the 4-byte account ID echo that rAthena sends
	// before any framed packets. accountID=2000002 from DUMP1 captures.
	// Source: char_clif.cpp:851-853 — WFIFOL(fd,0) = account_id; WFIFOSET(fd,4)
	if phaseIdx == 1 {
		var echo [4]byte
		binary.LittleEndian.PutUint32(echo[:], 2000002)
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := conn.Write(echo[:]); err != nil {
			return
		}
	}

	// Step 3: Drain remaining C→S bytes in a background goroutine so writes don't
	// deadlock. net.Pipe synchronises reads and writes; if the FSM sends C→S while
	// we're in a Write, the goroutine will consume them so the FSM isn't blocked.
	// Lifecycle: this goroutine exits when conn.Read returns an error, which happens
	// when defer conn.Close() fires at the end of servePhase. It is not explicitly
	// joined; it exits cleanly via the error return path.
	go func() {
		buf := make([]byte, 4096)
		for {
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Step 4: Write S→C bytes in random chunks.
	// For the map phase, use a small maximum chunk size (16 bytes) so the FSM
	// cannot consume all bytes — including post-OnReady packets — in a single
	// conn.Read call before OnReady fires and the test registers its handlers.
	// Without this, the entire 3857-byte map phase may be delivered in one chunk,
	// causing the FSM's feedUntil loop to process all packets (including 0x09FF)
	// before the test's handlers are registered in setupHandlers/OnReady.
	// Login and char phases use up to 4096 bytes — they have no post-OnReady issue.
	maxChunk := 4096
	if phaseIdx == 2 {
		maxChunk = 16
	}
	remaining := phaseData
	for len(remaining) > 0 {
		size := s.rng.Intn(maxChunk) + 1
		if size > len(remaining) {
			size = len(remaining)
		}
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := conn.Write(remaining[:size]); err != nil {
			return
		}
		remaining = remaining[size:]
	}

	// Give the FSM a moment to process the last bytes and send its C→S response.
	time.Sleep(50 * time.Millisecond)
	// Closing conn triggers the drain goroutine to exit.
}

// replayDialer is a Dialer that uses a ScriptedServer.
func replayDialer(ss *ScriptedServer) Dialer {
	return ss.Dialer()
}

// ── Fixture reading helpers ───────────────────────────────────────────────────

// mustLoadFixture loads a .fixture file or calls t.Fatal.
func mustLoadFixture(tb interface {
	Helper()
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}, path string) *fixtureData {
	tb.Helper()
	fix, err := loadFixture(path)
	if err != nil {
		tb.Fatalf("loadFixture(%q): %v", path, err)
	}
	return fix
}

// assertFixtureBytes verifies a known S→C packet is at the expected offset in the fixture.
// id is the expected packet ID; offset is byte position within the phase data.
func assertFixtureBytes(tb interface {
	Helper()
	Errorf(format string, args ...interface{})
}, data []byte, offset int, id uint16, label string) {
	tb.Helper()
	if offset+2 > len(data) {
		tb.Errorf("fixture check %s: offset %d out of range (len=%d)", label, offset, len(data))
		return
	}
	got := binary.LittleEndian.Uint16(data[offset : offset+2])
	if got != id {
		tb.Errorf("fixture check %s @ offset %d: got 0x%04X, want 0x%04X", label, offset, got, id)
	}
}

// ── I/O utility used in tests ────────────────────────────────────────────────

// pipeReadAll reads all available bytes from a net.Pipe connection until it
// is closed or a deadline expires. Used in tests to drain the map conn after
// OnReady fires.
func pipeReadAll(conn net.Conn, timeout time.Duration) []byte {
	var result []byte
	buf := make([]byte, 4096)
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	for time.Now().Before(deadline) {
		n, err := conn.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return result
}

// drainConn reads from conn until the deadline, discarding all bytes.
func drainConn(conn net.Conn, timeout time.Duration) {
	buf := make([]byte, 4096)
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for {
		_, err := conn.Read(buf)
		if err != nil {
			return
		}
	}
}

// ── io.ReadCloser wrapper ────────────────────────────────────────────────────

// ensure io is imported (used by pipeReadAll's net.Conn.Read)
var _ = io.EOF
