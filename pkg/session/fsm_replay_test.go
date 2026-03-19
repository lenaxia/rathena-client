// replay_test.go exercises the ConnectionFSM against captured rAthena wire bytes
// replayed by ScriptedServer. Tests run without a build tag — no Docker required.
//
// Fixture files in testdata/ were generated from real captures at packetver 20200401:
//   - auth_20200401.fixture   — DUMP1 (login + char + map entry)
//   - movement_20200401.fixture — DUMP8_movement (same auth + actor/movement)
//
// The tests verify:
//  1. OnReady fires (full three-phase auth completes)
//  2. Feed() does not fault during the fixture's S→C bytes
//  3. At least one expected event fires with a non-zero field
package session

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lenaxia/rathena-client/pkg/decode"
)

// testFixturePath returns the path to a fixture file in the package's testdata dir.
func testFixturePath(name string) string {
	return "testdata/" + name
}

// runReplayTest is the shared implementation for replay tests.
// It:
//  1. Loads the fixture
//  2. Drives ConnectionFSM.Connect() against a ScriptedServer
//  3. Calls setupHandlers(mapSess) to register event handlers
//  4. Feeds the map phase bytes remaining after OnReady fires
//  5. Returns the mapSess and conn for the caller to assert on
//
// The caller receives the mapSess and conn from OnReady. conn is already
// exhausted of fixture bytes (up to the timeout). The caller asserts its own
// gotXxx flags, which must have been set inside setupHandlers closures.
func runReplayTest(
	t *testing.T,
	fixtureName string,
	setupHandlers func(sess *MapSession),
	postReadyAction func(sess *MapSession, conn net.Conn),
) {
	t.Helper()
	fix := mustLoadFixture(t, testFixturePath(fixtureName))

	// Verify the first packet of each phase matches what we expect from the dumps.
	assertFixtureBytes(t, fix.login, 0, 0x0AC4, "login[0]")
	assertFixtureBytes(t, fix.char, 0, 0x082D, "char[0]")
	assertFixtureBytes(t, fix.mapPhase, 0, 0x0283, "map[0]")

	seed := seedFromName(t.Name())
	ss := newScriptedServer(fix, seed)

	server := ServerConfig{
		LoginAddr:   "unused",
		Packetver:   fix.packetver,
		StepTimeout: 10 * time.Second,
	}
	creds := Credentials{
		Username: "botijo1",
		Password: "Melon.77",
		CharSlot: 0,
	}

	type readyResult struct {
		sess *MapSession
		conn net.Conn
	}
	readyCh := make(chan readyResult, 1)

	f := New(server, creds, ss.Dialer()).
		OnCharServerList(func(_ []CharServerInfo) int { return 0 }).
		OnCharList(func(_ []byte) uint8 { return 0 }).
		OnReady(func(s *MapSession, c net.Conn, _ ReadyInfo) {
			setupHandlers(s)
			readyCh <- readyResult{s, c}
		}).
		OnFailed(func(err error) {
			t.Errorf("OnFailed: %v", err)
		})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := f.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case r := <-readyCh:
		if postReadyAction != nil {
			postReadyAction(r.sess, r.conn)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for OnReady")
	}
}

// TestReplay_FullAuth_20200401 replays auth_20200401.fixture and asserts that
// after OnReady fires, at least one stat_update or actor_exists event fires
// from the initial map burst.
//
// Fixture source: ~/personal/goKore/docs/03_REFERENCE/dumps/DUMP1
func TestReplay_FullAuth_20200401(t *testing.T) {
	var gotStatUpdate bool
	var gotActorExists bool
	var feedErrors int

	runReplayTest(t, "auth_20200401.fixture",
		func(sess *MapSession) {
			// 0x07FB ZC_USESKILL_CASTINIT — skill cast begins.
			// lengths_map.go sets this to 0 for pv >= 20191120 (disabled in
			// modern packets.hpp), but the real packetver 20200401 server sends it.
			// Observed in DUMP1: 25 bytes. GCC-verified against rAthena clif source.
			// Source: clif.cpp clif_skillcasting — WFIFOW(fd,0)=0x07FB; size=25
			sess.setLength(0x07FB, 25)
			sess.registerHandler(0x00B0, func(data []byte, pv uint32) {
				e := decode.StatUpdate_0x00B0(data, pv)
				if e.VarID != 0 || e.Count != 0 {
					gotStatUpdate = true
				}
			})
			sess.registerHandler(0x00B1, func(data []byte, pv uint32) {
				_ = decode.StatUpdate_0x00B1(data, pv)
				gotStatUpdate = true
			})
			sess.registerHandler(0x09FF, func(data []byte, pv uint32) {
				e := decode.ActorExists_0x09FF(data, pv)
				if e.GID != 0 {
					gotActorExists = true
				}
			})
			sess.registerHandler(0x0078, func(data []byte, pv uint32) {
				e := decode.ActorExists_0x0078(data, pv)
				if e.GID != 0 {
					gotActorExists = true
				}
			})
		},
		func(sess *MapSession, conn net.Conn) {
			// Read and feed all remaining S→C bytes from the map connection.
			buf := make([]byte, 4096)
			deadline := time.Now().Add(5 * time.Second)
			_ = conn.SetDeadline(deadline)
			for time.Now().Before(deadline) {
				n, err := conn.Read(buf)
				if n > 0 {
					if ferr := sess.Feed(buf[:n]); ferr != nil {
						feedErrors++
					}
				}
				if err != nil {
					break
				}
			}
			conn.Close()
			t.Logf("auth replay: gotStatUpdate=%v gotActorExists=%v feedErrors=%d",
				gotStatUpdate, gotActorExists, feedErrors)
		},
	)

	if feedErrors > 0 {
		t.Errorf("Feed() returned %d error(s) during replay — unknown packet lengths?", feedErrors)
	}
	if !gotStatUpdate && !gotActorExists {
		t.Error("no stat_update or actor_exists event fired during auth fixture replay")
	}
}

// TestReplay_Movement_20200401 replays movement_20200401.fixture and asserts
// that actor_exists events fire from the initial map burst.
//
// Fixture source: ~/personal/goKore/docs/03_REFERENCE/dumps/DUMP8_movement
func TestReplay_Movement_20200401(t *testing.T) {
	var gotActorExists bool
	var gotStatUpdate bool
	var feedErrors int

	runReplayTest(t, "movement_20200401.fixture",
		func(sess *MapSession) {
			sess.registerHandler(0x09FF, func(data []byte, pv uint32) {
				e := decode.ActorExists_0x09FF(data, pv)
				if e.GID != 0 {
					gotActorExists = true
				}
			})
			sess.registerHandler(0x0078, func(data []byte, pv uint32) {
				e := decode.ActorExists_0x0078(data, pv)
				if e.GID != 0 {
					gotActorExists = true
				}
			})
			sess.registerHandler(0x00B0, func(data []byte, pv uint32) {
				e := decode.StatUpdate_0x00B0(data, pv)
				if e.VarID != 0 || e.Count != 0 {
					gotStatUpdate = true
				}
			})
		},
		func(sess *MapSession, conn net.Conn) {
			buf := make([]byte, 4096)
			deadline := time.Now().Add(5 * time.Second)
			_ = conn.SetDeadline(deadline)
			for time.Now().Before(deadline) {
				n, err := conn.Read(buf)
				if n > 0 {
					if ferr := sess.Feed(buf[:n]); ferr != nil {
						feedErrors++
						t.Logf("Feed error (movement): %v", ferr)
					}
				}
				if err != nil {
					break
				}
			}
			conn.Close()
			t.Logf("movement replay: gotActorExists=%v gotStatUpdate=%v feedErrors=%d",
				gotActorExists, gotStatUpdate, feedErrors)
		},
	)

	if feedErrors > 0 {
		t.Errorf("Feed() returned %d error(s) during replay — unknown packet lengths?", feedErrors)
	}
	if !gotActorExists && !gotStatUpdate {
		t.Error("no actor_exists or stat_update event fired during movement fixture replay")
	}
}
