//go:build integration

// Package fsm_test contains the live server integration test for ConnectionFSM.
// Run with: go test -tags integration -timeout 60s -v ./pkg/fsm/ -run TestLiveServer
package fsm_test

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/lenaxia/ragnarok-go-client/pkg/decode"
	"github.com/lenaxia/ragnarok-go-client/pkg/fsm"
	"github.com/lenaxia/ragnarok-go-client/pkg/session"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// registerMapBurstLengths registers S→C packet lengths that are absent from
// lengths_map.go but arrive during the map-entry burst at packetver 20200401.
// Every entry is verified against DUMP8_movement and rAthena source where possible.
// Without these, MapSession.Feed() faults on the first unknown-length packet and
// silently drops all subsequent bytes — making event assertions impossible.
//
// Fixed-length sources:
//
//	0x007F ZC_NOTIFY_TIME:           packets.hpp  int16+uint32 = 6; GCC-verified
//	0x0087 ZC_NOTIFY_PLAYERMOVE:     packets.hpp  int16+uint32+uint8[6] = 12; GCC-verified
//	0x0091 ZC_NPCACK_MAPMOVE:        packets.hpp  int16+char[16]+uint16+uint16 = 22; GCC-verified
//	0x00B0 ZC_PAR_CHANGE:            packets_struct.hpp int16+uint16+int32 = 8; GCC-verified
//	0x00BD ZC_STATUS:                packets.hpp  int16+uint16+12×uint8+14×int16 = 44; GCC-verified
//	0x013A ZC_ATTACK_RANGE:          packets_struct.hpp int16+uint16 = 4; GCC-verified
//	0x0141 ZC_COUPLESTATUS:          packets_struct.hpp int16+uint32+int32+int32 = 14; GCC-verified
//	0x01D7 ZC_SPRITE_CHANGE:         packets_struct.hpp int16+uint32+uint8+uint32+uint32 = 15 (pv>=20181121); GCC-verified
//	0x02C9 ZC_PARTY_CONFIG:          packets_struct.hpp int16+uint8 = 3; GCC-verified
//	0x02D9 ZC_CONFIG:                clif_packetdb.hpp packet(0x02d9,10) = 10; DUMP8-verified
//	0x02DA ZC_CONFIG_NOTIFY:         packets_struct.hpp int16+uint8 = 3; GCC-verified
//	0x099B ZC_MAPPROPERTY_R2:        packets_struct.hpp int16+uint16+uint32 = 8; GCC-verified
//	0x09E7 ZC_NOTIFY_UNREAD_RODEX:   packets_struct.hpp int16+uint8 = 3; GCC-verified
//	0x0A24 ZC_ACH_UPDATE:            clif_packetdb.hpp packet(0x0A24,66) = 66; DUMP8-verified
//	0x0A9B (variable, see below):    clif_packetdb.hpp packet(0x0A9B,-1); 4 bytes in DUMP8
//	0x0ACB ZC_LONGLONGPAR_CHANGE:    packets_struct.hpp int16+uint16+int64 = 12; GCC-verified
//	0x0ADE ZC_OVERWEIGHT_PERCENT:    packets_struct.hpp int16+uint32 = 6; GCC-verified
//	0x0ADF ZC_ACK_REQNAMEALL_NPC:    packets_struct.hpp uint16+int32+int32+24+24 = 58; DUMP8-verified
//	0x0B0B ZC_INVENTORY_END:         packets_struct.hpp int16+uint8+char = 4; GCC-verified
//	0x0B1B ZC_NOTIFY_ACTORINIT:      packets_struct.hpp int16 = 2; GCC-verified
//	0x0B20 ZC_SHORTCUT_KEY_LIST:     packets_struct.hpp int16+int8+int16+(38×7) = 271 (pv>=20190522); GCC-verified
//
// Variable-length sources (len field at bytes 2-3, value -1):
//
//	0x008E ZC_NOTIFY_CHAT:           variable (two sizes in DUMP8: 64, 72)
//	0x010F ZC_SKILLINFO_LIST:        packets_struct.hpp has packetLength field
//	0x0A9B ZC_EXTEND_BODYITEM_SIZE_NOTIFY: clif_packetdb.hpp packet(0x0A9B,-1); 4 bytes in DUMP8 (header-only)
//	0x0B08 ZC_INVENTORY_START:       packets_struct.hpp has char name[]
//	0x0B09 ZC_INVENTORY_ITEMLIST_NORMAL: variable (DUMP8: 73 bytes)
//	0x0B0A ZC_INVENTORY_ITEMLIST_EQUIP:  variable (DUMP8: 608 bytes)
//	0x0A23 ZC_ACHIEVEMENT_LIST:      variable (DUMP8: 472 bytes)
func registerMapBurstLengths(s *session.MapSession) {
	// Fixed-length packets
	s.SetLength(0x007F, 6)   // ZC_NOTIFY_TIME: int16+uint32; GCC-verified=6
	s.SetLength(0x0087, 12)  // ZC_NOTIFY_PLAYERMOVE: int16+uint32+uint8[6]; GCC-verified=12
	s.SetLength(0x0091, 22)  // ZC_NPCACK_MAPMOVE: int16+char[16]+uint16+uint16; GCC-verified=22
	s.SetLength(0x00B0, 8)   // ZC_PAR_CHANGE: int16+uint16+int32; GCC-verified=8
	s.SetLength(0x00BD, 44)  // ZC_STATUS: int16+uint16+12×uint8+14×int16; GCC-verified=44
	s.SetLength(0x013A, 4)   // ZC_ATTACK_RANGE: int16+uint16; GCC-verified=4
	s.SetLength(0x0141, 14)  // ZC_COUPLESTATUS: int16+uint32+int32+int32; GCC-verified=14
	s.SetLength(0x01D7, 15)  // ZC_SPRITE_CHANGE: int16+uint32+uint8+uint32+uint32 (pv>=20181121); GCC-verified=15
	s.SetLength(0x02C9, 3)   // ZC_PARTY_CONFIG: int16+uint8; GCC-verified=3
	s.SetLength(0x02D9, 10)  // ZC_CONFIG: clif_packetdb.hpp packet(0x02d9,10); DUMP8-verified=10
	s.SetLength(0x02DA, 3)   // ZC_CONFIG_NOTIFY: int16+uint8; GCC-verified=3
	s.SetLength(0x099B, 8)   // ZC_MAPPROPERTY_R2: int16+uint16+uint32; GCC-verified=8
	s.SetLength(0x09E7, 3)   // ZC_NOTIFY_UNREAD_RODEX: int16+uint8; GCC-verified=3
	s.SetLength(0x0A24, 66)  // ZC_ACH_UPDATE: clif_packetdb.hpp packet(0x0A24,66); DUMP8-verified=66
	s.SetLength(0x0ACB, 12)  // ZC_LONGLONGPAR_CHANGE: int16+uint16+int64; GCC-verified=12
	s.SetLength(0x0ADE, 6)   // ZC_OVERWEIGHT_PERCENT: int16+uint32; GCC-verified=6
	s.SetLength(0x0ADF, 58)  // ZC_ACK_REQNAMEALL_NPC: uint16+int32+int32+24+24; DUMP8-verified=58
	s.SetLength(0x0B0B, 4)   // ZC_INVENTORY_END: int16+uint8+char; GCC-verified=4
	s.SetLength(0x0B1B, 2)   // ZC_NOTIFY_ACTORINIT: int16 only; GCC-verified=2
	s.SetLength(0x0B20, 271) // ZC_SHORTCUT_KEY_LIST: int16+int8+int16+(38×7) (pv>=20190522); GCC-verified=271

	// Variable-length packets (framer reads length from bytes[2:4])
	s.SetLength(0x008E, -1) // ZC_NOTIFY_CHAT: variable; 64 and 72 bytes seen in DUMP8
	s.SetLength(0x010F, -1) // ZC_SKILLINFO_LIST: has packetLength field; packets_struct.hpp:4279
	s.SetLength(0x0A23, -1) // ZC_ACHIEVEMENT_LIST: DUMP8 472 bytes (variable)
	s.SetLength(0x0A9B, -1) // ZC_EXTEND_BODYITEM_SIZE_NOTIFY: clif_packetdb.hpp packet(0x0A9B,-1); 4 bytes in DUMP8
	s.SetLength(0x0B08, -1) // ZC_INVENTORY_START: has char name[]; packets_struct.hpp:1232
	s.SetLength(0x0B09, -1) // ZC_INVENTORY_ITEMLIST_NORMAL: DUMP8 73 bytes (variable)
	s.SetLength(0x0B0A, -1) // ZC_INVENTORY_ITEMLIST_EQUIP: DUMP8 608 bytes (variable)
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

	server := fsm.ServerConfig{
		LoginAddr:   addr,
		Packetver:   pver,
		StepTimeout: 15 * time.Second,
	}
	creds := fsm.Credentials{
		Username: user,
		Password: pass,
		CharSlot: slot,
	}

	type readyResult struct {
		mapSess *session.MapSession
		conn    net.Conn
	}
	readyCh := make(chan readyResult, 1)

	f := fsm.New(server, creds, dialer).
		OnCharServerList(func(_ []fsm.CharServerInfo) int { return 0 }).
		OnCharList(func(_ []byte) uint8 { return slot }).
		OnReady(func(s *session.MapSession, c net.Conn) {
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

	r := <-readyCh

	// Register lengths for all S→C packets seen in the map-entry burst that
	// are absent from lengths_map.go. Must be done before any Feed() call —
	// the first unknown-length packet permanently faults the session.
	registerMapBurstLengths(r.mapSess)

	var gotActorExists, gotStatUpdate bool
	var feedErrors, feedCalls int

	r.mapSess.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
		e := decode.ActorExists_0x09FF(data, pv)
		if e.ID != 0 {
			gotActorExists = true
		}
	})
	r.mapSess.RegisterHandler(0x0078, func(data []byte, pv uint32) {
		e := decode.ActorExists_0x0078(data, pv)
		if e.ID != 0 {
			gotActorExists = true
		}
	})
	r.mapSess.RegisterHandler(0x00B0, func(data []byte, pv uint32) {
		e := decode.StatUpdate_0x00B0(data, pv)
		if e.StatType != 0 || e.Value != 0 {
			gotStatUpdate = true
		}
	})
	r.mapSess.RegisterHandler(0x00B1, func(data []byte, pv uint32) {
		e := decode.StatUpdate_0x00B1(data, pv)
		if e.StatType != 0 || e.Value != 0 {
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
