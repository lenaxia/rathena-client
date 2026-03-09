// Package fsm implements the ConnectionFSM that drives the full rAthena
// login → char → map authentication sequence.
//
// Design invariants (from HLD §4):
//   - Zero goroutines: Connect() runs synchronously in the caller's goroutine.
//   - Caller (goKore) provides a Dialer; the FSM never calls net.Dial directly.
//   - After OnReady fires, the FSM releases all references to net.Conn.
//   - StepTimeout is applied via conn.SetDeadline before each blocking read.
package fsm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/lenaxia/ragnarok-go-client/pkg/session"
)

// Dialer is provided by goKore. The FSM calls it for each of the three server
// connections. goKore may wrap any dialer it likes (direct, proxy, test stub).
type Dialer func(ctx context.Context, addr string) (net.Conn, error)

// ServerConfig holds the fixed properties of the game server.
// Shared across all bot instances connecting to the same server.
type ServerConfig struct {
	// LoginAddr is "host:port" of the rAthena login server.
	LoginAddr string
	// Packetver is the YYYYMMDD packet version; selects packet layouts and IDs.
	Packetver uint32
	// StepTimeout is the per-step deadline applied via conn.SetDeadline before
	// each blocking read inside Connect(). Defaults to 30s if zero.
	StepTimeout time.Duration
}

// Credentials holds the per-account authentication details.
type Credentials struct {
	Username string
	Password string
	// CharSlot is used when OnCharList is not registered.
	CharSlot uint8
}

// CharServerInfo describes a character server entry parsed from the login
// server's accept packet (0x0069 / 0x0AC4).
type CharServerInfo struct {
	IP   uint32
	Port uint16
	Name string
}

// CharacterInfo describes a character slot from the char server list.
// The raw bytes are passed to callers via RawChars because the CHARACTER_INFO
// struct layout varies by PACKETVER; goKore parses with its own codec.
type CharacterInfo struct {
	// Slot is the character's slot index.
	Slot uint8
}

// ConnectionFSM drives the full login → char → map auth sequence.
type ConnectionFSM struct {
	server ServerConfig
	creds  Credentials
	dialer Dialer

	onCharServerList func([]CharServerInfo) int
	onCharList       func([]byte) uint8
	onReady          func(*session.MapSession, net.Conn)
	onFailed         func(error)
	onServerNotify   func(uint8)

	// auth state — populated during Connect, cleared when Connect returns
	accountID  uint32
	sessionID1 uint32
	sessionID2 uint32
	sex        uint8
	charID     uint32

	// readBuf is reused across feedStep calls to avoid per-read heap allocations.
	readBuf [4096]byte
}

// New creates a ConnectionFSM. server and creds are stored for every Connect
// call (including reconnects). Changing either requires creating a new FSM.
func New(server ServerConfig, creds Credentials, dialer Dialer) *ConnectionFSM {
	if server.StepTimeout == 0 {
		server.StepTimeout = 30 * time.Second
	}
	return &ConnectionFSM{
		server: server,
		creds:  creds,
		dialer: dialer,
	}
}

// OnCharServerList registers a callback invoked after the login server accept
// packet is received. The callback receives all advertised char servers and
// returns the index to connect to. If not registered: index 0 is used.
func (f *ConnectionFSM) OnCharServerList(fn func([]CharServerInfo) int) *ConnectionFSM {
	f.onCharServerList = fn
	return f
}

// OnCharList registers a callback invoked when the full char list has been
// assembled. Receives raw CHARACTER_INFO bytes (variable struct size per
// PACKETVER); returns the slot number to select. If not registered:
// Credentials.CharSlot is used.
func (f *ConnectionFSM) OnCharList(fn func([]byte) uint8) *ConnectionFSM {
	f.onCharList = fn
	return f
}

// OnReady registers a callback invoked when the map server accepts entry
// (0x0073 / 0x0A18 / 0x02EB) and the map-loaded sequence has been sent.
// The FSM passes the ready MapSession and live net.Conn to goKore.
// After this call returns the FSM is idle and holds no reference to conn.
func (f *ConnectionFSM) OnReady(fn func(*session.MapSession, net.Conn)) *ConnectionFSM {
	f.onReady = fn
	return f
}

// OnFailed registers a callback invoked on any unrecoverable error.
func (f *ConnectionFSM) OnFailed(fn func(error)) *ConnectionFSM {
	f.onFailed = fn
	return f
}

// OnServerNotify registers a callback invoked when 0x0081 SC_NOTIFY_BAN is
// received during auth. The argument is the result/error code byte.
func (f *ConnectionFSM) OnServerNotify(fn func(uint8)) *ConnectionFSM {
	f.onServerNotify = fn
	return f
}

// Connect runs the full login sequence synchronously. It blocks until OnReady
// or OnFailed fires, then returns. It may be called multiple times on the same
// FSM for reconnection.
//
// If Connect encounters a failure it calls OnFailed(err) (if registered) and
// returns that same error. Callers should use EITHER the OnFailed callback OR
// the return value, not both.
func (f *ConnectionFSM) Connect(ctx context.Context) error {
	// Reset auth state from any previous run.
	f.accountID = 0
	f.sessionID1 = 0
	f.sessionID2 = 0
	f.sex = 0
	f.charID = 0

	err := f.connect(ctx)
	if err != nil {
		if f.onFailed != nil {
			f.onFailed(err)
		}
	}
	return err
}

func (f *ConnectionFSM) stepTimeout() time.Duration {
	if f.server.StepTimeout > 0 {
		return f.server.StepTimeout
	}
	return 30 * time.Second
}

// connect is the internal implementation of Connect (no OnFailed dispatch).
func (f *ConnectionFSM) connect(ctx context.Context) error {
	// ── Phase 1: Login server ────────────────────────────────────────────────

	charServers, charIdx, err := f.runLoginPhase(ctx)
	if err != nil {
		return err
	}
	if charIdx < 0 || charIdx >= len(charServers) {
		return fmt.Errorf("fsm: char server index %d out of range (have %d)", charIdx, len(charServers))
	}

	chosen := charServers[charIdx]
	charAddr := fmt.Sprintf("%d.%d.%d.%d:%d",
		chosen.IP>>24, (chosen.IP>>16)&0xFF, (chosen.IP>>8)&0xFF, chosen.IP&0xFF,
		chosen.Port)

	// ── Phase 2: Char server ─────────────────────────────────────────────────

	mapAddr, err := f.runCharPhase(ctx, charAddr)
	if err != nil {
		return err
	}

	// ── Phase 3: Map server ──────────────────────────────────────────────────

	return f.runMapPhase(ctx, mapAddr)
}

// ── Login phase ──────────────────────────────────────────────────────────────

// runLoginPhase dials the login server, sends 0x0064 CA_LOGIN, and reads until
// 0x0069/0x0AC4 (accepted) or 0x006A/0x083E/0x0081 (refused).
// Returns the parsed char server list and the chosen index.
func (f *ConnectionFSM) runLoginPhase(ctx context.Context) ([]CharServerInfo, int, error) {
	conn, err := f.dialer(ctx, f.server.LoginAddr)
	if err != nil {
		return nil, 0, fmt.Errorf("fsm: dial login %s: %w", f.server.LoginAddr, err)
	}
	defer conn.Close()

	pkt := buildLoginPacket(f.server.Packetver, f.creds.Username, f.creds.Password)
	if err := writeDeadline(conn, pkt, f.stepTimeout()); err != nil {
		return nil, 0, fmt.Errorf("fsm: send login: %w", err)
	}

	// Determine which accept packet ID we expect.
	acceptID := uint16(0x0069)
	if f.server.Packetver >= 20170315 {
		acceptID = 0x0AC4
	}

	loginSess := session.NewLoginSession(f.server.Packetver)

	var charServers []CharServerInfo
	done := false
	var loginErr error

	loginSess.RegisterHandler(acceptID, func(data []byte, _ uint32) {
		cs, aid, sid1, sid2, sx, err := parseLoginAccept(data, f.server.Packetver)
		if err != nil {
			loginErr = err
			done = true
			return
		}
		f.accountID = aid
		f.sessionID1 = sid1
		f.sessionID2 = sid2
		f.sex = sx
		charServers = cs
		done = true
	})

	// Login refused (legacy 0x006A — only active for pv < 20120000; for pv >= 20120000 it's 0x083E)
	for _, refuseID := range []uint16{0x006A, 0x083E} {
		refuseID := refuseID
		loginSess.RegisterHandler(refuseID, func(data []byte, _ uint32) {
			code := byte(0)
			if len(data) >= 3 {
				if refuseID == 0x006A {
					code = data[2]
				} else {
					// 0x083E: int16+uint32+char[20] — error in bytes 2-5
					if len(data) >= 6 {
						code = data[2]
					}
				}
			}
			loginErr = fmt.Errorf("fsm: login refused (code=%d)", code)
			done = true
		})
	}

	loginSess.RegisterHandler(0x0081, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		if f.onServerNotify != nil {
			f.onServerNotify(code)
		}
		loginErr = fmt.Errorf("fsm: server notify ban (code=%d)", code)
		done = true
	})

	if err := f.feedUntil(conn, loginSess, f.stepTimeout(), &done); err != nil {
		return nil, 0, fmt.Errorf("fsm: login feed: %w", err)
	}
	if loginErr != nil {
		return nil, 0, loginErr
	}

	idx := 0
	if f.onCharServerList != nil {
		idx = f.onCharServerList(charServers)
	}
	return charServers, idx, nil
}

// ── Char phase ───────────────────────────────────────────────────────────────

// charPhaseResult collects state during char auth.
type charPhaseResult struct {
	mapIP   uint32
	mapPort uint16
	charID  uint32
	done    bool
	err     error

	// char list accumulation
	rawChars     []byte // accumulated CHARACTER_INFO bytes
	charTotal    uint32 // total characters expected (from 0x09A0)
	pagesTotal   uint32 // total pages expected (from 0x09A0)
	pagesRecv    uint32 // pages received so far
	got09A0      bool   // whether we received HC_CHARLIST_NOTIFY
	gotCharList  bool   // whether char list is complete and OnCharList was called
	selectedSlot uint8
}

// runCharPhase dials the char server, drives the char auth state machine, and
// returns the map address "ip:port" extracted from HC_NOTIFY_ZONESVR.
func (f *ConnectionFSM) runCharPhase(ctx context.Context, charAddr string) (string, error) {
	conn, err := f.dialer(ctx, charAddr)
	if err != nil {
		return "", fmt.Errorf("fsm: dial char %s: %w", charAddr, err)
	}
	defer conn.Close()

	// Send 0x0065 CH_ENTER: accountID + sessionID1 + sessionID2 + clienttype + sex
	enterPkt := buildCharEnterPacket(f.accountID, f.sessionID1, f.sessionID2, f.sex)
	if err := writeDeadline(conn, enterPkt, f.stepTimeout()); err != nil {
		return "", fmt.Errorf("fsm: send char enter: %w", err)
	}

	charSess := session.NewCharSession(f.server.Packetver)

	// For PACKETVER < 20170315, 0x0081 is used as HC_NOTIFY_ZONESVR (28 bytes fixed)
	// AND as SC_NOTIFY_BAN (3 bytes). The char lengths table defaults to 3 (SC_NOTIFY_BAN).
	// Override to 28 so the framing engine consumes a full HC_NOTIFY_ZONESVR frame.
	// SC_NOTIFY_BAN arriving on the char connection during SelectingChar state will also
	// be framed as 28 bytes — the handler reads only byte[2] (result) and ignores the rest,
	// which is safe since SC_NOTIFY_BAN is always followed by connection close.
	// Source: common/packets.hpp PACKET_HC_NOTIFY_ZONESVR = 28 bytes at PACKETVER=20120000.
	if f.server.Packetver < 20170315 {
		charSess.SetLength(0x0081, 28)
	}

	res := &charPhaseResult{}

	// 0x006C HC_REFUSE_ENTER — refused
	charSess.RegisterHandler(0x006C, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		res.err = fmt.Errorf("fsm: char server refused entry (code=%d)", code)
		res.done = true
	})

	// 0x0081 on char server — SC_NOTIFY_BAN or (for pv < 20170315) HC_NOTIFY_ZONESVR
	// Disambiguate by payload size: SC_NOTIFY_BAN = 3 bytes; HC_NOTIFY_ZONESVR = 28 bytes.
	charSess.RegisterHandler(0x0081, func(data []byte, _ uint32) {
		if len(data) >= 28 {
			// HC_NOTIFY_ZONESVR (PACKETVER < 20170315)
			// struct: int16 + uint32 CID + char mapname[16] + uint32 ip + uint16 port
			// Source: common/packets.hpp PACKET_HC_NOTIFY_ZONESVR at PACKETVER=20120000
			cid := binary.LittleEndian.Uint32(data[2:6])
			// rAthena writes: p.ip = htonl(...) — network byte order (big-endian).
			// Source: char_clif.cpp:909 "p.ip = htonl(...)"
			ip := binary.BigEndian.Uint32(data[22:26])
			port := binary.LittleEndian.Uint16(data[26:28])
			res.charID = cid
			res.mapIP = ip
			res.mapPort = port
			f.charID = cid
			res.done = true
			return
		}
		// SC_NOTIFY_BAN
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		if f.onServerNotify != nil {
			f.onServerNotify(code)
		}
		res.err = fmt.Errorf("fsm: char server notify ban (code=%d)", code)
		res.done = true
	})

	// 0x0AC5 HC_NOTIFY_ZONESVR (PACKETVER >= 20170315)
	// struct: int16 + uint32 CID + char mapname[16] + uint32 ip + uint16 port + char domain[128]
	// Source: common/packets.hpp at PACKETVER=20180307, size=156
	charSess.RegisterHandler(0x0AC5, func(data []byte, _ uint32) {
		if len(data) < 28 {
			res.err = fmt.Errorf("fsm: 0x0AC5 too short (%d bytes)", len(data))
			res.done = true
			return
		}
		cid := binary.LittleEndian.Uint32(data[2:6])
		// rAthena writes: p.ip = htonl(...) — network byte order (big-endian).
		// Source: char_clif.cpp:909 "p.ip = htonl(...)"
		ip := binary.BigEndian.Uint32(data[22:26])
		port := binary.LittleEndian.Uint16(data[26:28])
		res.charID = cid
		res.mapIP = ip
		res.mapPort = port
		f.charID = cid
		res.done = true
	})

	// 0x082D HC_ACCEPT_ENTER2 (slot info, PACKETVER >= 20130000) — store and stay
	// struct: int16 + int16 packetLength + uint8 total + uint8 premium_start +
	//         uint8 premium_end + char extension[20] + CHARACTER_INFO characters[]
	// We do not parse CHARACTER_INFO here; just note we received it.
	charSess.RegisterHandler(0x082D, func(data []byte, _ uint32) {
		// Stay — char list will arrive later via 0x006B or paged via 0x09A0+0x099D
	})

	// 0x006B HC_ACCEPT_ENTER (char list)
	charSess.RegisterHandler(0x006B, func(data []byte, _ uint32) {
		if f.server.Packetver < 20130000 {
			// Pre-20130000: char list arrives directly in this packet; call OnCharList now.
			// struct: int16 + int16 packetLength + uint8 total + uint8 premium_start +
			//         uint8 premium_end + char extension[20] + CHARACTER_INFO characters[]
			// Header is 27 bytes: 2+2+1+1+1+20 = 27.
			rawChars := []byte{}
			if len(data) > 27 {
				rawChars = data[27:]
			}
			slot := f.pickCharSlot(rawChars)
			res.selectedSlot = slot
			res.gotCharList = true
		} else {
			// >= 20130000: accumulate chars; wait for 0x09A0 to know when complete.
			if len(data) > 27 {
				res.rawChars = append(res.rawChars, data[27:]...)
			}
		}
	})

	// 0x09A0 HC_CHARLIST_NOTIFY: total characters and pages
	// struct: int16 + uint32 total
	// Source: common/packets.hpp PACKET_HC_CHARLIST_NOTIFY, size=6
	charSess.RegisterHandler(0x09A0, func(data []byte, pv uint32) {
		if len(data) < 6 {
			return
		}
		total := binary.LittleEndian.Uint32(data[2:6])
		res.charTotal = total
		res.pagesTotal = total // rAthena sends one page per char in practice; we treat total == pages
		res.got09A0 = true

		// Send CH_CHARLIST_REQ (0x09A1) — one per page
		// struct: int16 packetType only (size=2)
		// Source: common/packets.hpp PACKET_CH_CHARLIST_REQ, size=2
		for i := uint32(0); i < total; i++ {
			pkt := buildCharlistReq()
			_ = writeDeadline(conn, pkt, f.stepTimeout())
		}
	})

	// 0x099D HC_ACK_CHARINFO_PER_PAGE: one page of char data
	// struct: int16 + int16 packetLength + CHARACTER_INFO characters[]
	// Header = 4 bytes.
	charSess.RegisterHandler(0x099D, func(data []byte, _ uint32) {
		if len(data) > 4 {
			res.rawChars = append(res.rawChars, data[4:]...)
		}
		res.pagesRecv++
		if res.got09A0 && res.pagesRecv >= res.pagesTotal {
			slot := f.pickCharSlot(res.rawChars)
			res.selectedSlot = slot
			res.gotCharList = true
		}
	})

	// Inner feed loop: runs until charList ready, then sends 0x0066, then waits for zone info.
	charDone := &res.done
	slotSent := false

	for !res.done {
		if res.gotCharList && !slotSent {
			// Send 0x0066 CH_SELECT_CHAR
			selPkt := buildSelectCharPacket(res.selectedSlot)
			if err := writeDeadline(conn, selPkt, f.stepTimeout()); err != nil {
				return "", fmt.Errorf("fsm: send select char: %w", err)
			}
			slotSent = true
		}

		if err := f.feedStep(conn, charSess, f.stepTimeout()); err != nil {
			return "", fmt.Errorf("fsm: char feed: %w", err)
		}
		_ = charDone
	}

	if res.err != nil {
		return "", res.err
	}

	mapAddr := fmt.Sprintf("%d.%d.%d.%d:%d",
		res.mapIP>>24, (res.mapIP>>16)&0xFF, (res.mapIP>>8)&0xFF, res.mapIP&0xFF,
		res.mapPort)
	return mapAddr, nil
}

// pickCharSlot calls OnCharList if registered, else uses Credentials.CharSlot.
func (f *ConnectionFSM) pickCharSlot(rawChars []byte) uint8 {
	if f.onCharList != nil {
		return f.onCharList(rawChars)
	}
	return f.creds.CharSlot
}

// ── Map phase ────────────────────────────────────────────────────────────────

// runMapPhase dials the map server, sends 0x0436 CZ_ENTER, reads until
// 0x0073/0x02EB/0x0A18 ZC_ACCEPT_ENTER, sends 0x007D + 0x007E/0x0360,
// then calls OnReady.
func (f *ConnectionFSM) runMapPhase(ctx context.Context, mapAddr string) error {
	conn, err := f.dialer(ctx, mapAddr)
	if err != nil {
		return fmt.Errorf("fsm: dial map %s: %w", mapAddr, err)
	}

	mapSess := session.NewMapSession(f.server.Packetver)

	// Register auth-phase packet lengths that may be absent from the generated
	// lengths_map.go (lengths_map.go is still partially populated — US-02).
	// These are the only packets the FSM receives before handing off to goKore.
	//
	// ZC_AID (0x0283): int16 + uint32 = 6 bytes; always present (pv >= 20070521)
	// Source: clif.cpp WFIFOW(fd,0)=0x283; clif_packetdb.hpp: packet(0x0283,6)
	mapSess.SetLength(0x0283, 6)

	// ZC_REFUSE_ENTER (0x0074): int16 + uint8 = 3 bytes
	// Source: packets.hpp DEFINE_PACKET_HEADER(ZC_REFUSE_ENTER, 0x74)
	mapSess.SetLength(0x0074, 3)

	// ZC_ACCEPT_ENTER variants — exactly one is used depending on packetver:
	// 0x0073 (< 20080102): int16+uint32+[3]uint8+uint8+uint8 = 11 bytes
	// 0x02EB (>= 20080102, not 20141022..20160329): +uint16 font = 13 bytes
	// 0x0A18 (20141022..20160329): +uint16+uint8 sex = 14 bytes
	// Source: packets.hpp PACKET_ZC_ACCEPT_ENTER / DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER,...)
	mapSess.SetLength(0x0073, 11)
	mapSess.SetLength(0x02EB, 13)
	mapSess.SetLength(0x0A18, 14)

	// ZC_INVENTORY_EXPANSION_INFO (0x0B18): int16 + uint16 size = 4 bytes.
	// Sent between ZC_AID and ZC_ACCEPT_ENTER in the map-entry burst.
	// NOT in lengths_map.go (US-02 incomplete); must be registered here so
	// feedUntil() can frame it before ZC_ACCEPT_ENTER arrives.
	// Source: DUMP1 line ~156; DUMP8_movement line ~156; packet is always 4 bytes.
	mapSess.SetLength(0x0B18, 4)

	// Enable C→S obfuscation if keys exist for this packetver.
	k0, k1, k2 := session.ObfuscationKeysFor(f.server.Packetver)
	if k0|k1|k2 != 0 {
		mapSess.EnableObfuscation(k0, k1, k2)
	}

	// Send 0x0436 CZ_ENTER
	// struct: int16 + uint32 AID + uint32 CID + uint32 login_id1 + uint32 clientTick + uint8 sex
	// Source: clif.cpp:10641 CZ_ENTER2
	enterPkt := buildMapEnterPacket(f.accountID, f.charID, f.sessionID1, f.sex)
	encodePacketID(mapSess, enterPkt)
	if err := writeDeadline(conn, enterPkt, f.stepTimeout()); err != nil {
		conn.Close()
		return fmt.Errorf("fsm: send map enter: %w", err)
	}

	type mapResult struct {
		done bool
		err  error
	}
	res := &mapResult{}

	// 0x0283 ZC_AID: account ID echo (PACKETVER >= 20070521)
	// struct: int16 + uint32 AID — size=6
	// Source: clif.cpp:10731 WFIFOW(fd,0)=0x283; WFIFOL(fd,2)=sd->id
	mapSess.RegisterHandler(0x0283, func(data []byte, _ uint32) {
		// Store if needed; currently we already have accountID from login.
	})

	// 0x0074 ZC_REFUSE_ENTER
	mapSess.RegisterHandler(0x0074, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		res.err = fmt.Errorf("fsm: map server refused entry (code=%d)", code)
		res.done = true
	})

	// 0x0081 SC_NOTIFY_BAN (on map server)
	mapSess.RegisterHandler(0x0081, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		if f.onServerNotify != nil {
			f.onServerNotify(code)
		}
		res.err = fmt.Errorf("fsm: map server ban (code=%d)", code)
		res.done = true
	})

	// ZC_ACCEPT_ENTER handler — fires on 0x0073, 0x02EB, or 0x0A18 depending on packetver.
	// Source: packets.hpp:546-575
	// struct (0x0073): int16 + uint32 startTime + uint8[3] posDir + uint8 xSize + uint8 ySize = 11 bytes
	// struct (0x02EB): + uint16 font = 13 bytes
	// struct (0x0A18): + uint8 sex = 14 bytes
	onMapEnter := func(data []byte, _ uint32) {
		if res.done {
			return
		}
		// Send 0x007D CZ_NOTIFY_ACTORINIT (map loaded confirmation)
		// struct: int16 only = 2 bytes; Source: clif.cpp:10742
		loadedPkt := buildMapLoadedPacket()
		encodePacketID(mapSess, loadedPkt)
		if err := writeDeadline(conn, loadedPkt, f.stepTimeout()); err != nil {
			res.err = fmt.Errorf("fsm: send map loaded: %w", err)
			res.done = true
			return
		}

		// Send 0x007E or 0x0360 CZ_REQUEST_TIME (tick sync)
		// struct: int16 + uint32 clientTick = 6 bytes; Source: clif.cpp:11196-11197
		_, tickPkt := buildTickSyncPacket(f.server.Packetver)
		encodePacketID(mapSess, tickPkt)
		if err := writeDeadline(conn, tickPkt, f.stepTimeout()); err != nil {
			res.err = fmt.Errorf("fsm: send tick sync: %w", err)
			res.done = true
			return
		}

		res.done = true
	}

	// Register the appropriate ZC_ACCEPT_ENTER packet ID based on packetver.
	// Source: packets.hpp #if PACKETVER conditions
	if f.server.Packetver >= 20141016 && f.server.Packetver < 20160330 {
		mapSess.RegisterHandler(0x0A18, onMapEnter) // ZC_ACCEPT_ENTER3
	} else if f.server.Packetver >= 20080102 {
		mapSess.RegisterHandler(0x02EB, onMapEnter) // ZC_ACCEPT_ENTER2
	} else {
		mapSess.RegisterHandler(0x0073, onMapEnter) // ZC_ACCEPT_ENTER (original)
	}

	if err := f.feedUntil(conn, mapSess, f.stepTimeout(), &res.done); err != nil {
		conn.Close()
		return fmt.Errorf("fsm: map feed: %w", err)
	}
	if res.err != nil {
		conn.Close()
		return res.err
	}

	// Hand off the conn and mapSess to goKore via OnReady.
	// After this call the FSM releases conn — goKore owns it.
	if f.onReady != nil {
		f.onReady(mapSess, conn)
	}
	// Do NOT close conn here — goKore owns it from this point.
	return nil
}

// ── I/O helpers ──────────────────────────────────────────────────────────────

// encodePacketID applies C→S obfuscation to the packet ID field (bytes 0-1) of
// pkt in-place. If obfuscation is not enabled on s, this is a no-op.
// pkt must be at least 2 bytes.
func encodePacketID(s *session.MapSession, pkt []byte) {
	id := binary.LittleEndian.Uint16(pkt[0:2])
	s.Encode(&id)
	binary.LittleEndian.PutUint16(pkt[0:2], id)
}

// writeDeadline writes b to conn with a per-operation deadline.
func writeDeadline(conn net.Conn, b []byte, timeout time.Duration) error {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	_, err := conn.Write(b)
	return err
}

// feedUntil reads from conn into sess.Feed() until *done is true or an error
// occurs. It sets a fresh deadline before each read.
func (f *ConnectionFSM) feedUntil(conn net.Conn, sess feeder, timeout time.Duration, done *bool) error {
	for !*done {
		if err := f.feedStep(conn, sess, timeout); err != nil {
			return err
		}
	}
	return nil
}

// feedStep performs one read + Feed cycle using the FSM's reusable read buffer.
func (f *ConnectionFSM) feedStep(conn net.Conn, sess feeder, timeout time.Duration) error {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	n, err := conn.Read(f.readBuf[:])
	if n > 0 {
		if ferr := sess.Feed(f.readBuf[:n]); ferr != nil {
			var unk session.ErrUnknownPacket
			if errors.As(ferr, &unk) {
				return fmt.Errorf("fsm: unknown packet 0x%04x", unk.ID)
			}
			return ferr
		}
	}
	if err != nil {
		return err
	}
	return nil
}

// feeder is the minimal interface satisfied by LoginSession, CharSession, MapSession.
type feeder interface {
	Feed([]byte) error
}
