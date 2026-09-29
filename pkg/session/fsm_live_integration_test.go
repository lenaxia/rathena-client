//go:build integration

// Contains the live server integration test for ConnectionFSM.
// Run with: go test -tags integration -timeout 60s -v ./pkg/session/ -run TestLiveServer
package session

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/lenaxia/rathena-client/pkg/decode"
	"github.com/lenaxia/rathena-client/pkg/events"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestLiveServer_FullAuthSequence(t *testing.T) {
	addr := envOrDefault("RATHENA_ADDR", "127.0.0.1:6900")
	pverStr := envOrDefault("RATHENA_PACKETVER", "20200401")
	user := envOrDefault("RATHENA_USER", "botijo1")
	pass := envOrDefault("RATHENA_PASS", "Melon.77")
	slotStr := envOrDefault("RATHENA_CHARSLOT", "0")

	// Skip if the server is not reachable (CI without Docker).
	probe, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("rAthena server not reachable at %s: %v", addr, err)
	}
	probe.Close()

	pver64, _ := strconv.ParseUint(pverStr, 10, 32)
	slot64, _ := strconv.ParseUint(slotStr, 10, 8)
	pver := uint32(pver64)
	slot := uint8(slot64)

	dialer := func(ctx context.Context, a string) (net.Conn, error) {
		d := &net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, "tcp", a)
	}

	server := ServerConfig{
		LoginAddr:   addr,
		Packetver:   pver,
		StepTimeout: 15 * time.Second,
	}
	creds := Credentials{
		Username: user,
		Password: pass,
		CharSlot: slot,
	}

	type readyResult struct {
		mapSess *MapSession
		conn    net.Conn
	}
	readyCh := make(chan readyResult, 1)

	f := New(server, creds, dialer).
		OnCharServerList(func(_ []CharServerInfo) int { return 0 }).
		OnCharList(func(_ []events.CharacterInfoEntry) uint8 { return slot }).
		OnReady(func(s *MapSession, c net.Conn, _ ReadyInfo) {
			readyCh <- readyResult{s, c}
		}).
		OnFailed(func(fi FailInfo) {
			t.Errorf("OnFailed: phase=%v err=%v", fi.Phase, fi.Err)
		})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := f.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	r := <-readyCh

	var gotActorExists, gotStatUpdate bool
	var feedErrors, feedCalls int

	r.mapSess.registerHandler(0x09FF, func(data []byte, pv uint32) {
		e := decode.ActorExists_0x09FF(data, pv)
		if e.GID != 0 {
			gotActorExists = true
		}
	})
	r.mapSess.registerHandler(0x0078, func(data []byte, pv uint32) {
		e := decode.ActorExists_0x0078(data, pv)
		if e.GID != 0 {
			gotActorExists = true
		}
	})
	r.mapSess.registerHandler(0x00B0, func(data []byte, pv uint32) {
		e := decode.StatUpdate_0x00B0(data, pv)
		if e.VarID != 0 || e.Count != 0 {
			gotStatUpdate = true
		}
	})
	r.mapSess.registerHandler(0x00B1, func(data []byte, pv uint32) {
		e := decode.StatUpdate_0x00B1(data, pv)
		if e.VarID != 0 || e.Count != 0 {
			gotStatUpdate = true
		}
	})

	buf := make([]byte, 4096)
	deadline := time.Now().Add(5 * time.Second)
	r.conn.SetDeadline(deadline)

	for time.Now().Before(deadline) {
		n, readErr := r.conn.Read(buf)
		if n > 0 {
			feedCalls++
			if feedErr := r.mapSess.Feed(buf[:n]); feedErr != nil {
				t.Errorf("Feed error after %d calls: %v", feedCalls, feedErr)
				feedErrors++
				break
			}
		}
		if readErr != nil {
			break
		}
	}
	r.conn.Close()

	t.Logf("Feed calls: %d, feed errors: %d, gotActorExists: %v, gotStatUpdate: %v",
		feedCalls, feedErrors, gotActorExists, gotStatUpdate)

	if !gotActorExists && !gotStatUpdate {
		t.Error("no actor_exists or stat_update event fired in 5-second window")
	}
}
