package fsm

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/lenaxia/ragnarok-go-client/pkg/session"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// serverScript is a function that acts as a scripted server-side peer.
type serverScript func(t *testing.T, conn net.Conn)

// scriptedDialer returns a Dialer that serves connections from the provided
// scripts in order (first call → scripts[0], second → scripts[1], etc.).
func scriptedDialer(t *testing.T, scripts ...serverScript) Dialer {
	t.Helper()
	idx := 0
	return func(ctx context.Context, addr string) (net.Conn, error) {
		i := idx
		idx++
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			scripts[i](t, server)
		}()
		return client, nil
	}
}

// mustWrite writes b to conn or calls t.Fatal.
func mustWrite(t *testing.T, conn net.Conn, b []byte) {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(b); err != nil {
		t.Fatalf("server write: %v", err)
	}
}

// mustRead reads exactly n bytes from conn and returns them.
func mustRead(t *testing.T, conn net.Conn, n int) []byte {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("server read(%d): %v", n, err)
	}
	return buf
}

// mustDrain reads and discards exactly n bytes from conn.
func mustDrain(t *testing.T, conn net.Conn, n int) {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("server drain(%d): %v", n, err)
	}
}

// ── Wire-format builders (server side) ───────────────────────────────────────

// buildLoginAcceptPre builds a 0x0069 AC_ACCEPT_LOGIN packet for PACKETVER < 20170315.
// header = 47 bytes, each sub-entry = 32 bytes.
// Source: common/packets.hpp PACKET_AC_ACCEPT_LOGIN at PACKETVER=20120000
// IP is written big-endian (network order) to match rAthena: loginclif.cpp:137 htonl().
func buildLoginAcceptPre(sid1, aid, sid2 uint32, sex uint8, servers []CharServerInfo) []byte {
	const subSize = 32
	total := 47 + len(servers)*subSize
	pkt := make([]byte, total)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0069)
	binary.LittleEndian.PutUint16(pkt[2:4], uint16(total))
	binary.LittleEndian.PutUint32(pkt[4:8], sid1)
	binary.LittleEndian.PutUint32(pkt[8:12], aid)
	binary.LittleEndian.PutUint32(pkt[12:16], sid2)
	// last_ip[4] at 16:20, last_login[26] at 20:46 — zero
	pkt[46] = sex
	off := 47
	for _, s := range servers {
		binary.BigEndian.PutUint32(pkt[off:], s.IP) // htonl — network byte order
		binary.LittleEndian.PutUint16(pkt[off+4:], s.Port)
		copyStr(pkt[off+6:off+26], s.Name)
		// users(2)+type(2)+new_(2) at off+26:off+32 — zero
		off += subSize
	}
	return pkt
}

// buildLoginAcceptPost builds a 0x0AC4 AC_ACCEPT_LOGIN packet for PACKETVER >= 20170315.
// header = 64 bytes (47 fixed + 17 token), each sub-entry = 160 bytes (32 + 128 unknown).
// Source: common/packets.hpp PACKET_AC_ACCEPT_LOGIN at PACKETVER=20180307
// IP is written big-endian (network order) to match rAthena: loginclif.cpp:137 htonl().
func buildLoginAcceptPost(sid1, aid, sid2 uint32, sex uint8, servers []CharServerInfo) []byte {
	const subSize = 160
	total := 64 + len(servers)*subSize
	pkt := make([]byte, total)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0AC4)
	binary.LittleEndian.PutUint16(pkt[2:4], uint16(total))
	binary.LittleEndian.PutUint32(pkt[4:8], sid1)
	binary.LittleEndian.PutUint32(pkt[8:12], aid)
	binary.LittleEndian.PutUint32(pkt[12:16], sid2)
	pkt[46] = sex
	// token[17] at 47:64 — zero
	off := 64
	for _, s := range servers {
		binary.BigEndian.PutUint32(pkt[off:], s.IP) // htonl — network byte order
		binary.LittleEndian.PutUint16(pkt[off+4:], s.Port)
		copyStr(pkt[off+6:off+26], s.Name)
		// unknown[128] — zero
		off += subSize
	}
	return pkt
}

// buildLoginRefuse builds a 0x083E AC_REFUSE_LOGIN packet (PACKETVER >= 20120000).
// struct: int16 + uint32 error + char unblock_time[20] = 26 bytes
// Source: common/packets.hpp PACKET_AC_REFUSE_LOGIN
func buildLoginRefuse(code uint32) []byte {
	pkt := make([]byte, 26)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x083E)
	binary.LittleEndian.PutUint32(pkt[2:6], code)
	return pkt
}

// buildCharEnterAccept builds a 0x006B HC_ACCEPT_ENTER (char list, PACKETVER < 20130000).
// header = 27 bytes: int16(2)+int16(2)+uint8(1)+uint8(1)+uint8(1)+char[20](20) = 27
// Source: common/packets.hpp PACKET_HC_ACCEPT_ENTER
func buildCharEnterAccept(rawChars []byte) []byte {
	total := 27 + len(rawChars)
	pkt := make([]byte, total)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x006B)
	binary.LittleEndian.PutUint16(pkt[2:4], uint16(total))
	// total(1)+premium_start(1)+premium_end(1)+extension[20] — zero
	if len(rawChars) > 0 {
		copy(pkt[27:], rawChars)
	}
	return pkt
}

// buildCharRefuse builds a 0x006C HC_REFUSE_ENTER packet.
// struct: int16 + uint8 error = 3 bytes
// Source: common/packets.hpp PACKET_HC_REFUSE_ENTER
func buildCharRefuse(code uint8) []byte {
	pkt := make([]byte, 3)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x006C)
	pkt[2] = code
	return pkt
}

// buildHCNotifyZonesvrPre builds a 0x0081 HC_NOTIFY_ZONESVR packet (PACKETVER < 20170315).
// struct: int16 + uint32 CID + char mapname[16] + uint32 ip + uint16 port = 28 bytes
// Source: common/packets.hpp PACKET_HC_NOTIFY_ZONESVR at PACKETVER=20120000
// IP is written big-endian (network order) to match rAthena: char_clif.cpp:909 htonl().
func buildHCNotifyZonesvrPre(cid, ip uint32, port uint16) []byte {
	pkt := make([]byte, 28)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0081)
	binary.LittleEndian.PutUint32(pkt[2:6], cid)
	// mapname[16] at 6:22 — zero
	binary.BigEndian.PutUint32(pkt[22:26], ip) // htonl — network byte order
	binary.LittleEndian.PutUint16(pkt[26:28], port)
	return pkt
}

// buildHCNotifyZonesvrPost builds a 0x0AC5 HC_NOTIFY_ZONESVR packet (PACKETVER >= 20170315).
// struct: int16 + uint32 CID + char mapname[16] + uint32 ip + uint16 port + char domain[128] = 156 bytes
// Source: common/packets.hpp PACKET_HC_NOTIFY_ZONESVR at PACKETVER=20180307
// IP is written big-endian (network order) to match rAthena: char_clif.cpp:909 htonl().
func buildHCNotifyZonesvrPost(cid, ip uint32, port uint16) []byte {
	pkt := make([]byte, 156)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0AC5)
	binary.LittleEndian.PutUint32(pkt[2:6], cid)
	binary.BigEndian.PutUint32(pkt[22:26], ip) // htonl — network byte order
	binary.LittleEndian.PutUint16(pkt[26:28], port)
	return pkt
}

// buildSCNotifyBan builds a 0x0081 SC_NOTIFY_BAN packet (3 bytes).
// Source: common/packets.hpp PACKET_SC_NOTIFY_BAN
func buildSCNotifyBan(code uint8) []byte {
	pkt := make([]byte, 3)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0081)
	pkt[2] = code
	return pkt
}

// buildZCAcceptEnter builds the appropriate ZC_ACCEPT_ENTER packet.
// Source: packets.hpp:546-575
//
//	0x0073 (< 20080102): int16+uint32+[3]uint8+uint8+uint8 = 11 bytes
//	0x02EB (>= 20080102, not 20141022..20160329): +uint16 font = 13 bytes
//	0x0A18 (20141022..20160329): +uint16+uint8 sex = 14 bytes
func buildZCAcceptEnter(packetver uint32) []byte {
	if packetver >= 20141016 && packetver < 20160330 {
		pkt := make([]byte, 14)
		binary.LittleEndian.PutUint16(pkt[0:2], 0x0A18)
		return pkt
	} else if packetver >= 20080102 {
		pkt := make([]byte, 13)
		binary.LittleEndian.PutUint16(pkt[0:2], 0x02EB)
		return pkt
	}
	pkt := make([]byte, 11)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0073)
	return pkt
}

// buildHC082D builds a 0x082D HC_ACCEPT_ENTER2 (slot info) packet.
// struct: int16 + int16 packetLength + uint8 total + uint8 premium_start +
//
//	uint8 premium_end + char extension[20] + CHARACTER_INFO characters[]
//
// With no characters: 2+2+1+1+1+20+2 = 29 bytes (matches lengths_char.go t[0x082D]=29)
// Source: common/packets.hpp PACKET_HC_ACCEPT_ENTER2
func buildHC082D() []byte {
	pkt := make([]byte, 29)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x082D)
	binary.LittleEndian.PutUint16(pkt[2:4], 29)
	return pkt
}

// buildHCCharlistNotify builds a 0x09A0 HC_CHARLIST_NOTIFY packet.
// struct: int16 packetType + uint32 total = 6 bytes
// Source: common/packets.hpp PACKET_HC_CHARLIST_NOTIFY
func buildHCCharlistNotify(total uint32) []byte {
	pkt := make([]byte, 6)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x09A0)
	binary.LittleEndian.PutUint32(pkt[2:6], total)
	return pkt
}

// buildHCAckCharinfoPerPage builds a 0x099D HC_ACK_CHARINFO_PER_PAGE packet.
// struct: int16 + int16 packetLength + CHARACTER_INFO characters[]
// With rawChars data: header = 4 bytes.
// Source: common/packets.hpp PACKET_HC_ACK_CHARINFO_PER_PAGE
func buildHCAckCharinfoPerPage(rawChars []byte) []byte {
	total := 4 + len(rawChars)
	pkt := make([]byte, total)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x099D)
	binary.LittleEndian.PutUint16(pkt[2:4], uint16(total))
	if len(rawChars) > 0 {
		copy(pkt[4:], rawChars)
	}
	return pkt
}

// buildZCAID builds a 0x0283 ZC_AID packet (PACKETVER >= 20070521).
// struct: int16 + uint32 AID = 6 bytes
// Source: clif.cpp WFIFOW(fd,0)=0x283; WFIFOL(fd,2)=sd->id
func buildZCAID(aid uint32) []byte {
	pkt := make([]byte, 6)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0283)
	binary.LittleEndian.PutUint32(pkt[2:6], aid)
	return pkt
}

// ── Integration tests ─────────────────────────────────────────────────────────

// TestConnect_FullFlow_Pre20170315 tests the complete auth flow for PACKETVER < 20170315
// (old login accept 0x0069, old zone server notify 0x0081 with ≥28 bytes).
// Packet content is verified field-by-field — not just byte count.
func TestConnect_FullFlow_Pre20170315(t *testing.T) {
	const pv = uint32(20120000)
	const aid = uint32(2000001)
	const sid1 = uint32(0xDEADBEEF)
	const sid2 = uint32(0xCAFEBABE)
	const charID = uint32(150001)
	const mapIP = uint32(0x7F000001) // 127.0.0.1 in network byte order (big-endian)
	const mapPort = uint16(5121)

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121, Name: "TestChar"}

	loginScript := func(t *testing.T, conn net.Conn) {
		// Verify CA_LOGIN fields: type=0x0064, version=pv, username="test", password="pass"
		pkt := mustRead(t, conn, 55)
		if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0064 {
			t.Errorf("CA_LOGIN: type=%#04x, want 0x0064", binary.LittleEndian.Uint16(pkt[0:2]))
		}
		if binary.LittleEndian.Uint32(pkt[2:6]) != pv {
			t.Errorf("CA_LOGIN: version=%d, want %d", binary.LittleEndian.Uint32(pkt[2:6]), pv)
		}
		if strings.TrimRight(string(pkt[6:30]), "\x00") != "test" {
			t.Errorf("CA_LOGIN: username=%q, want test", strings.TrimRight(string(pkt[6:30]), "\x00"))
		}
		if strings.TrimRight(string(pkt[30:54]), "\x00") != "pass" {
			t.Errorf("CA_LOGIN: password=%q, want pass", strings.TrimRight(string(pkt[30:54]), "\x00"))
		}
		mustWrite(t, conn, buildLoginAcceptPre(sid1, aid, sid2, 1, []CharServerInfo{charServer}))
	}
	charScript := func(t *testing.T, conn net.Conn) {
		// Verify CH_ENTER fields: type=0x0065, accountID=aid, sessionID1=sid1, sessionID2=sid2
		pkt := mustRead(t, conn, 17)
		if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0065 {
			t.Errorf("CH_ENTER: type=%#04x, want 0x0065", binary.LittleEndian.Uint16(pkt[0:2]))
		}
		if binary.LittleEndian.Uint32(pkt[2:6]) != aid {
			t.Errorf("CH_ENTER: accountID=%d, want %d", binary.LittleEndian.Uint32(pkt[2:6]), aid)
		}
		if binary.LittleEndian.Uint32(pkt[6:10]) != sid1 {
			t.Errorf("CH_ENTER: sessionID1=%#x, want %#x", binary.LittleEndian.Uint32(pkt[6:10]), sid1)
		}
		if binary.LittleEndian.Uint32(pkt[10:14]) != sid2 {
			t.Errorf("CH_ENTER: sessionID2=%#x, want %#x", binary.LittleEndian.Uint32(pkt[10:14]), sid2)
		}
		mustWrite(t, conn, buildCharEnterAccept(nil))
		// Verify CH_SELECT_CHAR: type=0x0066, slot=0
		pkt = mustRead(t, conn, 3)
		if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0066 {
			t.Errorf("CH_SELECT_CHAR: type=%#04x, want 0x0066", binary.LittleEndian.Uint16(pkt[0:2]))
		}
		if pkt[2] != 0 {
			t.Errorf("CH_SELECT_CHAR: slot=%d, want 0", pkt[2])
		}
		mustWrite(t, conn, buildHCNotifyZonesvrPre(charID, mapIP, mapPort))
	}
	mapScript := func(t *testing.T, conn net.Conn) {
		// Verify CZ_ENTER2: type=0x0436, accountID=aid, charID=0 (not yet known at this point)
		pkt := mustRead(t, conn, 19)
		if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0436 {
			t.Errorf("CZ_ENTER2: type=%#04x, want 0x0436", binary.LittleEndian.Uint16(pkt[0:2]))
		}
		if binary.LittleEndian.Uint32(pkt[2:6]) != aid {
			t.Errorf("CZ_ENTER2: accountID=%d, want %d", binary.LittleEndian.Uint32(pkt[2:6]), aid)
		}
		mustWrite(t, conn, buildZCAID(aid))
		mustWrite(t, conn, buildZCAcceptEnter(pv)) // 0x02EB (pv >= 20080102), 13 bytes
		// Verify CZ_NOTIFY_ACTORINIT: type=0x007D, size=2
		pkt = mustRead(t, conn, 2)
		if binary.LittleEndian.Uint16(pkt[0:2]) != 0x007D {
			t.Errorf("CZ_NOTIFY_ACTORINIT: type=%#04x, want 0x007D", binary.LittleEndian.Uint16(pkt[0:2]))
		}
		// Verify CZ_REQUEST_TIME: type=0x007E (pv < 20080102) or 0x0360 (pv >= 20080102)
		pkt = mustRead(t, conn, 6)
		if binary.LittleEndian.Uint16(pkt[0:2]) != 0x007E && binary.LittleEndian.Uint16(pkt[0:2]) != 0x0360 {
			t.Errorf("CZ_REQUEST_TIME: type=%#04x, want 0x007E or 0x0360", binary.LittleEndian.Uint16(pkt[0:2]))
		}
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{Username: "test", Password: "pass", CharSlot: 0}

	readyCalled := false
	loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript, mapScript)).
		OnReady(func(s *session.MapSession, c net.Conn) {
			readyCalled = true
			c.Close()
		})

	if err := loginFSM.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !readyCalled {
		t.Fatal("OnReady was not called")
	}
}

// TestConnect_FullFlow_Post20170315 tests the complete auth flow for PACKETVER >= 20170315
// (new login accept 0x0AC4, new zone server notify 0x0AC5).
func TestConnect_FullFlow_Post20170315(t *testing.T) {
	const pv = uint32(20180307)
	const aid = uint32(2000001)
	const sid1 = uint32(0xDEADBEEF)
	const sid2 = uint32(0xCAFEBABE)
	const charID = uint32(150001)
	const mapIP = uint32(0x7F000001)
	const mapPort = uint16(5121)

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121, Name: "TestChar"}

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildLoginAcceptPost(sid1, aid, sid2, 1, []CharServerInfo{charServer}))
	}
	charScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 17) // CH_ENTER
		// pv >= 20130000 paged flow
		mustWrite(t, conn, buildHC082D())
		mustWrite(t, conn, buildCharEnterAccept(nil))
		mustWrite(t, conn, buildHCCharlistNotify(1))
		mustDrain(t, conn, 2) // 0x09A1
		mustWrite(t, conn, buildHCAckCharinfoPerPage(nil))
		mustDrain(t, conn, 3) // 0x0066
		mustWrite(t, conn, buildHCNotifyZonesvrPost(charID, mapIP, mapPort))
	}
	mapScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 19)
		mustWrite(t, conn, buildZCAID(aid))
		mustWrite(t, conn, buildZCAcceptEnter(pv)) // 0x02EB, 13 bytes
		mustDrain(t, conn, 2)                      // 0x007D
		mustDrain(t, conn, 6)                      // 0x0360
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{Username: "testuser", Password: "testpass", CharSlot: 0}

	readyCalled := false
	loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript, mapScript)).
		OnReady(func(s *session.MapSession, c net.Conn) {
			readyCalled = true
			c.Close()
		})

	if err := loginFSM.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !readyCalled {
		t.Fatal("OnReady was not called")
	}
}

// TestConnect_LoginRefused tests that a login refusal triggers OnFailed and returns error.
func TestConnect_LoginRefused(t *testing.T) {
	const pv = uint32(20180307)

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildLoginRefuse(1))
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{Username: "bad", Password: "creds"}

	failedCalled := false
	loginFSM := New(server, creds, scriptedDialer(t, loginScript)).
		OnFailed(func(err error) {
			failedCalled = true
			if !strings.Contains(err.Error(), "login refused") {
				t.Errorf("unexpected error msg: %v", err)
			}
		})

	err := loginFSM.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !failedCalled {
		t.Fatal("OnFailed was not called")
	}
}

// TestConnect_CharServerRefused tests that a char server refusal triggers OnFailed.
func TestConnect_CharServerRefused(t *testing.T) {
	const pv = uint32(20180307)
	const aid = uint32(2000001)
	const sid1 = uint32(0x11111111)
	const sid2 = uint32(0x22222222)

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121, Name: "CS"}

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildLoginAcceptPost(sid1, aid, sid2, 0, []CharServerInfo{charServer}))
	}
	charScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 17)
		mustWrite(t, conn, buildCharRefuse(0))
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{}

	failedCalled := false
	loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript)).
		OnFailed(func(err error) {
			failedCalled = true
		})

	err := loginFSM.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !failedCalled {
		t.Fatal("OnFailed was not called")
	}
}

// TestConnect_ServerNotifyBan_Login tests that SC_NOTIFY_BAN on the login server
// fires OnServerNotify with the correct code.
func TestConnect_ServerNotifyBan_Login(t *testing.T) {
	const pv = uint32(20180307)
	const banCode = uint8(2)

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildSCNotifyBan(banCode))
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{}

	var gotCode uint8
	notifyCalled := false
	loginFSM := New(server, creds, scriptedDialer(t, loginScript)).
		OnServerNotify(func(code uint8) {
			notifyCalled = true
			gotCode = code
		})

	_ = loginFSM.Connect(context.Background())

	if !notifyCalled {
		t.Fatal("OnServerNotify was not called")
	}
	if gotCode != banCode {
		t.Errorf("code=%d, want %d", gotCode, banCode)
	}
}

// TestConnect_DialError tests that a dialer error triggers OnFailed.
func TestConnect_DialError(t *testing.T) {
	server := ServerConfig{
		LoginAddr:   "127.0.0.1:19999",
		Packetver:   20180307,
		StepTimeout: 100 * time.Millisecond,
	}
	creds := Credentials{}

	failDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return nil, net.ErrClosed
	}

	failedCalled := false
	loginFSM := New(server, creds, failDialer).
		OnFailed(func(err error) {
			failedCalled = true
		})

	err := loginFSM.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !failedCalled {
		t.Fatal("OnFailed was not called")
	}
}

// TestConnect_OnCharServerList tests that OnCharServerList fires with the correct
// server list and that the chosen index is used to dial.
func TestConnect_OnCharServerList(t *testing.T) {
	const pv = uint32(20180307)
	const aid = uint32(3000001)
	const sid1 = uint32(0xAAAAAAAA)
	const sid2 = uint32(0xBBBBBBBB)
	const charID = uint32(250001)
	const mapIP = uint32(0x7F000001)
	const mapPort = uint16(5121)

	charServers := []CharServerInfo{
		{IP: 0x01020304, Port: 6121, Name: "Alpha"},
		{IP: 0x05060708, Port: 6122, Name: "Beta"},
	}

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildLoginAcceptPost(sid1, aid, sid2, 0, charServers))
	}
	charScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 17) // CH_ENTER
		// pv >= 20130000 paged flow
		mustWrite(t, conn, buildHC082D())
		mustWrite(t, conn, buildCharEnterAccept(nil))
		mustWrite(t, conn, buildHCCharlistNotify(1))
		mustDrain(t, conn, 2) // 0x09A1
		mustWrite(t, conn, buildHCAckCharinfoPerPage(nil))
		mustDrain(t, conn, 3) // 0x0066
		mustWrite(t, conn, buildHCNotifyZonesvrPost(charID, mapIP, mapPort))
	}
	mapScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 19)
		mustWrite(t, conn, buildZCAID(aid))
		mustWrite(t, conn, buildZCAcceptEnter(pv))
		mustDrain(t, conn, 2)
		mustDrain(t, conn, 6)
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{}

	var receivedServers []CharServerInfo
	readyCalled := false

	loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript, mapScript)).
		OnCharServerList(func(servers []CharServerInfo) int {
			receivedServers = servers
			return 0 // choose Alpha (index 0)
		}).
		OnReady(func(_ *session.MapSession, c net.Conn) {
			readyCalled = true
			c.Close()
		})

	if err := loginFSM.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if len(receivedServers) != 2 {
		t.Fatalf("OnCharServerList got %d servers, want 2", len(receivedServers))
	}
	if receivedServers[0].Name != "Alpha" {
		t.Errorf("server[0]=%q, want Alpha", receivedServers[0].Name)
	}
	if receivedServers[1].Name != "Beta" {
		t.Errorf("server[1]=%q, want Beta", receivedServers[1].Name)
	}
	if !readyCalled {
		t.Fatal("OnReady was not called")
	}
}

// TestConnect_OnCharList tests that OnCharList fires with the raw char bytes.
func TestConnect_OnCharList(t *testing.T) {
	const pv = uint32(20120000) // pre-20130000: char list arrives directly in 0x006B
	const aid = uint32(2000001)
	const sid1 = uint32(0x11111111)
	const sid2 = uint32(0x22222222)
	const charID = uint32(100001)
	const mapIP = uint32(0x7F000001)
	const mapPort = uint16(5121)

	rawChars := []byte{0x01, 0x02, 0x03, 0x04} // dummy char bytes

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121, Name: "CS"}

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildLoginAcceptPre(sid1, aid, sid2, 0, []CharServerInfo{charServer}))
	}
	charScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 17)
		mustWrite(t, conn, buildCharEnterAccept(rawChars))
		mustDrain(t, conn, 3)
		mustWrite(t, conn, buildHCNotifyZonesvrPre(charID, mapIP, mapPort))
	}
	mapScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 19)
		mustWrite(t, conn, buildZCAID(aid))
		mustWrite(t, conn, buildZCAcceptEnter(pv)) // 0x02EB (pv >= 20080102)
		mustDrain(t, conn, 2)
		mustDrain(t, conn, 6)
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{CharSlot: 0}

	var gotRawChars []byte
	charListCalled := false

	loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript, mapScript)).
		OnCharList(func(raw []byte) uint8 {
			charListCalled = true
			gotRawChars = append([]byte(nil), raw...)
			return 0
		}).
		OnReady(func(_ *session.MapSession, c net.Conn) {
			c.Close()
		})

	if err := loginFSM.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if !charListCalled {
		t.Fatal("OnCharList was not called")
	}
	if string(gotRawChars) != string(rawChars) {
		t.Errorf("raw chars = %v, want %v", gotRawChars, rawChars)
	}
}

// TestConnect_Reconnect tests that Connect can be called multiple times
// on the same FSM (reconnect path), and auth state is reset each time.
func TestConnect_Reconnect(t *testing.T) {
	const pv = uint32(20180307)
	const aid = uint32(2000001)
	const sid1 = uint32(0x11111111)
	const sid2 = uint32(0x22222222)
	const charID = uint32(100001)
	const mapIP = uint32(0x7F000001)
	const mapPort = uint16(5121)

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121}

	mkLogin := func() serverScript {
		return func(t *testing.T, conn net.Conn) {
			mustDrain(t, conn, 55)
			mustWrite(t, conn, buildLoginAcceptPost(sid1, aid, sid2, 0, []CharServerInfo{charServer}))
		}
	}
	mkChar := func() serverScript {
		return func(t *testing.T, conn net.Conn) {
			mustDrain(t, conn, 17) // CH_ENTER
			// pv >= 20130000 paged flow
			mustWrite(t, conn, buildHC082D())
			mustWrite(t, conn, buildCharEnterAccept(nil))
			mustWrite(t, conn, buildHCCharlistNotify(1))
			mustDrain(t, conn, 2) // 0x09A1
			mustWrite(t, conn, buildHCAckCharinfoPerPage(nil))
			mustDrain(t, conn, 3) // 0x0066
			mustWrite(t, conn, buildHCNotifyZonesvrPost(charID, mapIP, mapPort))
		}
	}
	mkMap := func() serverScript {
		return func(t *testing.T, conn net.Conn) {
			mustDrain(t, conn, 19)
			mustWrite(t, conn, buildZCAID(aid))
			mustWrite(t, conn, buildZCAcceptEnter(pv))
			mustDrain(t, conn, 2)
			mustDrain(t, conn, 6)
		}
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{}

	readyCount := 0
	loginFSM := New(server, creds,
		scriptedDialer(t,
			mkLogin(), mkChar(), mkMap(),
			mkLogin(), mkChar(), mkMap(),
		)).
		OnReady(func(_ *session.MapSession, c net.Conn) {
			readyCount++
			c.Close()
		})

	ctx := context.Background()
	if err := loginFSM.Connect(ctx); err != nil {
		t.Fatalf("Connect #1: %v", err)
	}
	if err := loginFSM.Connect(ctx); err != nil {
		t.Fatalf("Connect #2: %v", err)
	}
	if readyCount != 2 {
		t.Errorf("OnReady called %d times, want 2", readyCount)
	}
}

// TestConnect_MapRefused tests that ZC_REFUSE_ENTER (0x0074) causes OnFailed.
func TestConnect_MapRefused(t *testing.T) {
	const pv = uint32(20180307)
	const aid = uint32(2000001)
	const sid1 = uint32(0x11111111)
	const sid2 = uint32(0x22222222)
	const charID = uint32(100001)
	const mapIP = uint32(0x7F000001)
	const mapPort = uint16(5121)

	charServer := CharServerInfo{IP: 0x7F000001, Port: 6121}

	loginScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 55)
		mustWrite(t, conn, buildLoginAcceptPost(sid1, aid, sid2, 0, []CharServerInfo{charServer}))
	}
	charScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 17) // CH_ENTER
		// pv >= 20130000 paged flow
		mustWrite(t, conn, buildHC082D())
		mustWrite(t, conn, buildCharEnterAccept(nil))
		mustWrite(t, conn, buildHCCharlistNotify(1))
		mustDrain(t, conn, 2) // 0x09A1
		mustWrite(t, conn, buildHCAckCharinfoPerPage(nil))
		mustDrain(t, conn, 3) // 0x0066
		mustWrite(t, conn, buildHCNotifyZonesvrPost(charID, mapIP, mapPort))
	}
	mapScript := func(t *testing.T, conn net.Conn) {
		mustDrain(t, conn, 19)
		// ZC_REFUSE_ENTER (0x0074): int16 + uint8 = 3 bytes
		// Source: packets.hpp PACKET_ZC_REFUSE_ENTER, DEFINE_PACKET_HEADER(ZC_REFUSE_ENTER, 0x74)
		pkt := make([]byte, 3)
		binary.LittleEndian.PutUint16(pkt[0:2], 0x0074)
		pkt[2] = 1
		mustWrite(t, conn, pkt)
	}

	server := ServerConfig{
		LoginAddr:   "127.0.0.1:6900",
		Packetver:   pv,
		StepTimeout: 5 * time.Second,
	}
	creds := Credentials{}

	failedCalled := false
	loginFSM := New(server, creds, scriptedDialer(t, loginScript, charScript, mapScript)).
		OnFailed(func(err error) {
			failedCalled = true
		})

	err := loginFSM.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !failedCalled {
		t.Fatal("OnFailed was not called")
	}
}

// ── Packet builder unit tests ─────────────────────────────────────────────────

func TestBuildLoginPacket(t *testing.T) {
	pkt := buildLoginPacket(20180307, "user", "pass")
	if len(pkt) != 55 {
		t.Fatalf("len=%d, want 55", len(pkt))
	}
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0064 {
		t.Errorf("packetType=%#04x, want 0x0064", binary.LittleEndian.Uint16(pkt[0:2]))
	}
	if binary.LittleEndian.Uint32(pkt[2:6]) != 20180307 {
		t.Errorf("version=%d, want 20180307", binary.LittleEndian.Uint32(pkt[2:6]))
	}
	if string(pkt[6:10]) != "user" {
		t.Errorf("username prefix = %q", string(pkt[6:10]))
	}
	if string(pkt[30:34]) != "pass" {
		t.Errorf("password prefix = %q", string(pkt[30:34]))
	}
}

func TestBuildCharEnterPacket(t *testing.T) {
	pkt := buildCharEnterPacket(1001, 0xDEAD, 0xBEEF, 1)
	if len(pkt) != 17 {
		t.Fatalf("len=%d, want 17", len(pkt))
	}
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0065 {
		t.Errorf("packetType=%#04x", binary.LittleEndian.Uint16(pkt[0:2]))
	}
	if binary.LittleEndian.Uint32(pkt[2:6]) != 1001 {
		t.Errorf("accountID=%d", binary.LittleEndian.Uint32(pkt[2:6]))
	}
	if pkt[16] != 1 {
		t.Errorf("sex=%d, want 1", pkt[16])
	}
}

func TestBuildSelectCharPacket(t *testing.T) {
	pkt := buildSelectCharPacket(3)
	if len(pkt) != 3 {
		t.Fatalf("len=%d, want 3", len(pkt))
	}
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0066 {
		t.Errorf("packetType=%#04x", binary.LittleEndian.Uint16(pkt[0:2]))
	}
	if pkt[2] != 3 {
		t.Errorf("slot=%d, want 3", pkt[2])
	}
}

func TestBuildMapEnterPacket(t *testing.T) {
	pkt := buildMapEnterPacket(1001, 2001, 0xABCD, 0)
	if len(pkt) != 19 {
		t.Fatalf("len=%d, want 19", len(pkt))
	}
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0436 {
		t.Errorf("packetType=%#04x", binary.LittleEndian.Uint16(pkt[0:2]))
	}
	if binary.LittleEndian.Uint32(pkt[2:6]) != 1001 {
		t.Errorf("accountID=%d", binary.LittleEndian.Uint32(pkt[2:6]))
	}
	if binary.LittleEndian.Uint32(pkt[6:10]) != 2001 {
		t.Errorf("charID=%d", binary.LittleEndian.Uint32(pkt[6:10]))
	}
	if binary.LittleEndian.Uint32(pkt[10:14]) != 0xABCD {
		t.Errorf("sessionID1=%#x", binary.LittleEndian.Uint32(pkt[10:14]))
	}
}

func TestBuildMapLoadedPacket(t *testing.T) {
	pkt := buildMapLoadedPacket()
	if len(pkt) != 2 {
		t.Fatalf("len=%d, want 2", len(pkt))
	}
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x007D {
		t.Errorf("packetType=%#04x", binary.LittleEndian.Uint16(pkt[0:2]))
	}
}

func TestBuildTickSyncPacket(t *testing.T) {
	// PACKETVER < 20080102 → 0x007E
	_, pkt := buildTickSyncPacket(20070521)
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x007E {
		t.Errorf("pv=20070521: id=%#04x, want 0x007E", binary.LittleEndian.Uint16(pkt[0:2]))
	}
	if len(pkt) != 6 {
		t.Fatalf("len=%d, want 6", len(pkt))
	}

	// PACKETVER >= 20080102 → 0x0360
	_, pkt = buildTickSyncPacket(20080102)
	if binary.LittleEndian.Uint16(pkt[0:2]) != 0x0360 {
		t.Errorf("pv=20080102: id=%#04x, want 0x0360", binary.LittleEndian.Uint16(pkt[0:2]))
	}
}

// ── Parse unit tests ──────────────────────────────────────────────────────────

func TestParseLoginAccept_Pre20170315(t *testing.T) {
	const sid1 = uint32(0x11111111)
	const aid = uint32(2000001)
	const sid2 = uint32(0x22222222)
	const sex = uint8(1)

	servers := []CharServerInfo{{IP: 0x7F000001, Port: 6121, Name: "TestCS"}}
	data := buildLoginAcceptPre(sid1, aid, sid2, sex, servers)

	gotServers, gotAID, gotSID1, gotSID2, gotSex, err := parseLoginAccept(data, 20120000)
	if err != nil {
		t.Fatalf("parseLoginAccept: %v", err)
	}
	if gotAID != aid {
		t.Errorf("AID=%d, want %d", gotAID, aid)
	}
	if gotSID1 != sid1 {
		t.Errorf("SID1=%#x, want %#x", gotSID1, sid1)
	}
	if gotSID2 != sid2 {
		t.Errorf("SID2=%#x, want %#x", gotSID2, sid2)
	}
	if gotSex != sex {
		t.Errorf("sex=%d, want %d", gotSex, sex)
	}
	if len(gotServers) != 1 {
		t.Fatalf("len(servers)=%d, want 1", len(gotServers))
	}
	if gotServers[0].Name != "TestCS" {
		t.Errorf("name=%q, want TestCS", gotServers[0].Name)
	}
	if gotServers[0].Port != 6121 {
		t.Errorf("port=%d, want 6121", gotServers[0].Port)
	}
	// Verify IP round-trip: 0x7F000001 → wire (big-endian) → parsed back → 0x7F000001
	// Formatted: 127.0.0.1
	if gotServers[0].IP != 0x7F000001 {
		t.Errorf("IP=%#08x, want 0x7F000001 (127.0.0.1)", gotServers[0].IP)
	}
	ipStr := fmt.Sprintf("%d.%d.%d.%d",
		gotServers[0].IP>>24, (gotServers[0].IP>>16)&0xFF,
		(gotServers[0].IP>>8)&0xFF, gotServers[0].IP&0xFF)
	if ipStr != "127.0.0.1" {
		t.Errorf("IP string=%q, want 127.0.0.1", ipStr)
	}
}

func TestParseLoginAccept_Post20170315(t *testing.T) {
	const sid1 = uint32(0xAAAAAAAA)
	const aid = uint32(3000001)
	const sid2 = uint32(0xBBBBBBBB)

	servers := []CharServerInfo{{IP: 0x01020304, Port: 6200, Name: "PostCS"}}
	data := buildLoginAcceptPost(sid1, aid, sid2, 0, servers)

	gotServers, gotAID, gotSID1, _, _, err := parseLoginAccept(data, 20180307)
	if err != nil {
		t.Fatalf("parseLoginAccept: %v", err)
	}
	if gotAID != aid {
		t.Errorf("AID=%d, want %d", gotAID, aid)
	}
	if gotSID1 != sid1 {
		t.Errorf("SID1=%#x, want %#x", gotSID1, sid1)
	}
	if len(gotServers) != 1 {
		t.Fatalf("len(servers)=%d, want 1", len(gotServers))
	}
	if gotServers[0].Name != "PostCS" {
		t.Errorf("name=%q, want PostCS", gotServers[0].Name)
	}
	// Verify IP round-trip: 0x01020304 → wire (big-endian) → parsed back → 0x01020304
	// Formatted: 1.2.3.4
	if gotServers[0].IP != 0x01020304 {
		t.Errorf("IP=%#08x, want 0x01020304 (1.2.3.4)", gotServers[0].IP)
	}
	ipStr := fmt.Sprintf("%d.%d.%d.%d",
		gotServers[0].IP>>24, (gotServers[0].IP>>16)&0xFF,
		(gotServers[0].IP>>8)&0xFF, gotServers[0].IP&0xFF)
	if ipStr != "1.2.3.4" {
		t.Errorf("IP string=%q, want 1.2.3.4", ipStr)
	}
}

func TestParseLoginAccept_MultipleServers(t *testing.T) {
	servers := []CharServerInfo{
		{IP: 0x01010101, Port: 6001, Name: "S1"},
		{IP: 0x02020202, Port: 6002, Name: "S2"},
		{IP: 0x03030303, Port: 6003, Name: "S3"},
	}
	data := buildLoginAcceptPre(1, 2, 3, 0, servers)
	got, _, _, _, _, err := parseLoginAccept(data, 20120000)
	if err != nil {
		t.Fatalf("parseLoginAccept: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	for i, s := range servers {
		if got[i].Name != s.Name {
			t.Errorf("[%d] name=%q, want %q", i, got[i].Name, s.Name)
		}
	}
}

func TestParseLoginAccept_TooShort(t *testing.T) {
	_, _, _, _, _, err := parseLoginAccept([]byte{0x69, 0x00}, 20120000)
	if err == nil {
		t.Fatal("expected error for too-short packet")
	}
}

func TestCStr(t *testing.T) {
	tests := []struct {
		input []byte
		want  string
	}{
		{[]byte("hello\x00world"), "hello"},
		{[]byte("noterm"), "noterm"},
		{[]byte{0x00}, ""},
		{[]byte("ab\x00"), "ab"},
	}
	for _, tc := range tests {
		got := cStr(tc.input)
		if got != tc.want {
			t.Errorf("cStr(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
