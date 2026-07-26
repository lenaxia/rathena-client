package session

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestConnect_MapLoadDelay_AppliedBeforeLoadEndAck verifies that when
// ServerConfig.MapLoadDelay is set, the FSM waits that duration between
// receiving ZC_ACCEPT_ENTER and sending CZ_NOTIFY_ACTORINIT (0x007D).
//
// This test measures the wall-clock time between the map server sending
// ZC_ACCEPT_ENTER and receiving the LoadEndAck (0x007D). With MapLoadDelay=0
// (default), this is <50ms. With MapLoadDelay=300ms, it should be >=300ms.
//
// Background: a normal client has a 200-500ms rendering delay between map
// entry and LoadEndAck. Bot frameworks that send LoadEndAck immediately can
// trigger server-side race conditions (rAthena SIGSEGV in status_calc_pc for
// Super Novice characters when PC_DIE_COUNTER register sync arrives before
// sd->bonus is populated). See ServerConfig.MapLoadDelay doc comment.
func TestConnect_MapLoadDelay_AppliedBeforeLoadEndAck(t *testing.T) {
	const pv = uint32(20180307)
	const aid = uint32(2000001)
	const sid1 = uint32(0xDEADBEEF)
	const sid2 = uint32(0xCAFEBABE)
	const charID = uint32(150001)
	const mapIP = uint32(0x7F000001)
	const mapPort = uint16(5121)

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121, Name: "TestChar"}

	tests := []struct {
		name       string
		delay      time.Duration
		minElapsed time.Duration // lower bound for LoadEndAck timing
	}{
		{name: "no_delay", delay: 0, minElapsed: 0},
		{name: "300ms_delay", delay: 300 * time.Millisecond, minElapsed: 250 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var loadEndAckElapsed time.Duration

			loginScript := func(t *testing.T, conn net.Conn) {
				mustDrain(t, conn, 55)
				mustWrite(t, conn, buildLoginAcceptPost(sid1, aid, sid2, 1, []CharServerInfo{charServer}))
			}
			charScript := func(t *testing.T, conn net.Conn) {
				mustDrain(t, conn, 17)
				writeAccountIDEcho(t, conn, aid)
				mustWrite(t, conn, buildHC082D())
				mustWrite(t, conn, buildCharEnterAccept(nil))
				mustWrite(t, conn, buildHCCharlistNotify(1))
				mustDrain(t, conn, 2)
				mustWrite(t, conn, buildHCAckCharinfoPerPage(nil))
				mustDrain(t, conn, 3)
				mustWrite(t, conn, buildHCNotifyZonesvrPost(charID, mapIP, mapPort))
			}
			mapScript := func(t *testing.T, conn net.Conn) {
				mustDrain(t, conn, 19)
				mustWrite(t, conn, buildZCAID(aid))

				// Record the time just before sending ZC_ACCEPT_ENTER.
				enterTime := time.Now()
				mustWrite(t, conn, buildZCAcceptEnterWithPos(0x12345678, 150, 200, 3, 7))

				// Read LoadEndAck (2 bytes) + TimeSyncResponse (6 bytes) = 8 bytes.
				mustRead(t, conn, 8)

				loadEndAckElapsed = time.Since(enterTime)
			}

			server := ServerConfig{
				LoginAddr:    "127.0.0.1:6900",
				Packetver:    pv,
				StepTimeout:  5 * time.Second,
				MapLoadDelay: tt.delay,
			}
			creds := Credentials{Username: "test", Password: "pass", CharSlot: 0}

			readyCalled := false
			loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript, mapScript)).
				OnReady(func(_ *MapSession, c net.Conn, info ReadyInfo) {
					readyCalled = true
					c.Close()
				})

			if err := loginFSM.Connect(context.Background()); err != nil {
				t.Fatalf("Connect: %v", err)
			}
			if !readyCalled {
				t.Fatal("OnReady was not called")
			}

			t.Logf("delay=%v: LoadEndAck elapsed=%v", tt.delay, loadEndAckElapsed)

			if loadEndAckElapsed < tt.minElapsed {
				t.Errorf("LoadEndAck arrived after %v, expected >= %v (delay=%v)",
					loadEndAckElapsed, tt.minElapsed, tt.delay)
			}
		})
	}
}
