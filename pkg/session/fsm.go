// ConnectionFSM and supporting types live here after the pkg/fsm merge.
// See the package-level doc comment in session.go for the full package description.
//
// Design invariants (from HLD §4):
//   - Zero goroutines: Connect() runs synchronously in the caller's goroutine.
//   - Caller (goKore) provides a Dialer; the FSM never calls net.Dial directly.
//   - After OnReady fires, the FSM releases all references to net.Conn.
//   - StepTimeout is applied via conn.SetDeadline before each blocking read.
//
// NOTE: This file intentionally does NOT import pkg/encode. pkg/encode imports
// pkg/session (for RegisterSendEncoder), so any session→encode import would create
// a cycle. Auth-phase packet encoding is done inline here for the small fixed-size
// packets that the FSM needs to send (CA_LOGIN, CH_ENTER, etc.).
package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/lenaxia/rathena-client/pkg/decode"
	"github.com/lenaxia/rathena-client/pkg/events"
	"github.com/lenaxia/rathena-client/pkg/packing"
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
// rAthena source: common/packets.hpp PACKET_AC_ACCEPT_LOGIN_sub
type CharServerInfo struct {
	// IP is the char server's IPv4 address in big-endian (network) byte order,
	// as written by rAthena's htonl() call.
	// To format as a dotted-decimal string:
	//   fmt.Sprintf("%d.%d.%d.%d", IP>>24, (IP>>16)&0xFF, (IP>>8)&0xFF, IP&0xFF)
	IP   uint32
	Port uint16
	Name string
	// Users is the current player count on this char server.
	// rAthena source: PACKET_AC_ACCEPT_LOGIN_sub.users (uint16)
	// OpenKore: "users" field in parse_account_server_info
	Users uint16
}

// CharacterInfo describes a character slot from the char server list.
// Deprecated: replaced by events.CharacterInfoEntry. Retained as an alias
// to avoid breaking callers until a clean-up pass removes all references.
// Use events.CharacterInfoEntry directly.

// IdentityInfo contains the player's identity after char selection completes.
// Passed to OnIdentity callback after the zone server response (0x0081/0x0AC5)
// is parsed, before the map phase begins.
type IdentityInfo struct {
	AccountID    uint32
	CharID       uint32
	SelectedSlot uint8
	Sex          uint8
	MapName      string // map name without .gat suffix, from HC_NOTIFY_ZONESVR
	MapIP        uint32 // map server IPv4 in host byte order (big-endian as sent by rAthena htonl)
	MapPort      uint16 // map server port
	// MapDomain is the optional CDN/proxy hostname from PACKET_HC_NOTIFY_ZONESVR.domain[128]
	// (present only when pv >= 20170315, i.e. packet 0x0AC5). Format is either a plain
	// hostname or "hostname:port". When non-empty, callers should prefer this over MapIP
	// for the map server address. OpenKore calls this field "mapUrl".
	// rAthena source: common/packets.hpp:297 char domain[128]
	MapDomain string
}

// AuthPhase identifies which authentication phase triggered a failure or
// server notification.
type AuthPhase uint8

const (
	PhaseLogin AuthPhase = iota // 0x0069 / 0x0AC4 login server phase
	PhaseChar                   // char server phase
	PhaseMap                    // map server phase
)

func (p AuthPhase) String() string {
	switch p {
	case PhaseLogin:
		return "login"
	case PhaseChar:
		return "char"
	case PhaseMap:
		return "map"
	default:
		return "unknown"
	}
}

// FailInfo is passed to the OnFailed callback on any unrecoverable FSM error.
type FailInfo struct {
	Phase AuthPhase // which phase failed
	Err   error     // underlying error
}

// NotifyInfo is passed to the OnServerNotify callback when SC_NOTIFY_BAN (0x0081)
// is received. rAthena source: common/packets.hpp:311–313 PACKET_SC_NOTIFY_BAN.
// OpenKore: Receive.pm "errors" sub — uses args->{type} directly to decide reconnect vs quit.
type NotifyInfo struct {
	Phase AuthPhase // which phase sent the notify
	Code  uint8     // rAthena: PACKET_SC_NOTIFY_BAN.result
}

// SlotInfo carries the character slot quota from HC_ACCEPT_ENTER2 (0x082D).
// Sent by the char server for PACKETVER >= 20130000 before the char list arrives.
// rAthena source: common/packets.hpp:508–517 PACKET_HC_ACCEPT_ENTER2
// OpenKore: Receive.pm $charSvrSet{normal_slot}, {premium_slot}, {billing_slot}, etc.
type SlotInfo struct {
	Normal     uint8 // rAthena: normal    — regular slot count
	Premium    uint8 // rAthena: premium   — premium slot count
	Billing    uint8 // rAthena: billing   — billing slot count
	Producible uint8 // rAthena: producible — producible slot count
	Total      uint8 // rAthena: total     — total slot count
}

// The FSM consumes the entry packet before handing off to goKore; these fields
// let the caller read the initial position without re-parsing a consumed frame.
type ReadyInfo struct {
	X         uint16 // initial tile X coordinate
	Y         uint16 // initial tile Y coordinate
	Dir       uint8  // facing direction (0–7)
	StartTime uint32 // server tick from entry packet
	Font      uint16 // overhead font ID
	Sex       uint8  // character sex byte
}

// ConnectionFSM drives the full login → char → map auth sequence.
type ConnectionFSM struct {
	server ServerConfig
	creds  Credentials
	dialer Dialer

	onCharServerList    func([]CharServerInfo) int
	onCharList          func([]events.CharacterInfoEntry) uint8
	onSlotInfo          func(SlotInfo)
	onMapSessionCreated func(*MapSession)
	onReady             func(*MapSession, net.Conn, ReadyInfo)
	onFailed            func(FailInfo)
	onServerNotify      func(NotifyInfo)
	onIdentity          func(IdentityInfo)

	// auth state — populated during Connect, cleared when Connect returns
	accountID  uint32
	sessionID1 uint32
	sessionID2 uint32
	sex        uint8
	charID     uint32

	// readBuf is reused across feedStep calls to avoid per-read heap allocations.
	readBuf [4096]byte
}

const defaultStepTimeout = 30 * time.Second

// New creates a ConnectionFSM. server and creds are stored for every Connect
// call (including reconnects). Changing either requires creating a new FSM.
func New(server ServerConfig, creds Credentials, dialer Dialer) *ConnectionFSM {
	if server.StepTimeout == 0 {
		server.StepTimeout = defaultStepTimeout
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

// OnSlotInfo registers a callback invoked when HC_ACCEPT_ENTER2 (0x082D) is
// received from the char server (PACKETVER >= 20130000). It carries slot quotas
// (normal/premium/billing/producible/total) before the character list arrives.
// rAthena source: common/packets.hpp:508–517 PACKET_HC_ACCEPT_ENTER2
func (f *ConnectionFSM) OnSlotInfo(fn func(SlotInfo)) *ConnectionFSM {
	f.onSlotInfo = fn
	return f
}

// OnCharList registers a callback invoked when the full char list has been
// assembled. Receives the parsed CHARACTER_INFO entries; returns the slot number
// to select. If not registered: Credentials.CharSlot is used.
func (f *ConnectionFSM) OnCharList(fn func([]events.CharacterInfoEntry) uint8) *ConnectionFSM {
	f.onCharList = fn
	return f
}

// OnMapSessionCreated registers fn to be called immediately after the MapSession
// is created for the map phase and the FSM's own internal auth handlers are
// registered, but before feedUntil processes any packets. This is the correct
// place to register semantic handlers that need to capture packets sent by the
// server as part of the initial map-login sequence (e.g., inventory burst, skill
// list, character stats broadcast sent in response to CZ_NOTIFY_ACTORINIT).
//
// fn is called synchronously from Connect(). It must not block.
// fn receives the MapSession fully configured (packetver, lengths, obfuscation,
// FSM auth handlers). Handlers registered here are keyed by SemanticAction and
// will not collide with the FSM's internal raw-ID handlers.
//
// The net.Conn is NOT available at this point (it is handed to the caller via
// OnReady). fn should only register receive-direction handlers.
func (f *ConnectionFSM) OnMapSessionCreated(fn func(*MapSession)) *ConnectionFSM {
	f.onMapSessionCreated = fn
	return f
}

// OnReady registers a callback invoked when the map server accepts entry
// (0x0073 / 0x0A18 / 0x02EB) and the map-loaded sequence has been sent.
// The FSM passes the ready MapSession, live net.Conn, and ReadyInfo (initial
// position decoded from ZC_ACCEPT_ENTER) to goKore.
// After this call returns the FSM is idle and holds no reference to conn.
func (f *ConnectionFSM) OnReady(fn func(*MapSession, net.Conn, ReadyInfo)) *ConnectionFSM {
	f.onReady = fn
	return f
}

// OnFailed registers a callback invoked on any unrecoverable error.
// The FailInfo carries the auth phase that failed and the underlying error.
func (f *ConnectionFSM) OnFailed(fn func(FailInfo)) *ConnectionFSM {
	f.onFailed = fn
	return f
}

// OnServerNotify registers a callback invoked when 0x0081 SC_NOTIFY_BAN is
// received from any server phase. The NotifyInfo carries the phase and the
// numeric ban/disconnect code. rAthena source: common/packets.hpp:311–313.
// OpenKore: Receive.pm "errors" sub — code meanings documented there.
func (f *ConnectionFSM) OnServerNotify(fn func(NotifyInfo)) *ConnectionFSM {
	f.onServerNotify = fn
	return f
}

// OnIdentity registers a callback invoked after char selection completes and
// the zone server response (0x0081/0x0AC5) is received. The callback receives
// AccountID, CharID, SelectedSlot, and Sex — all the identity info needed to
// initialize self-actor state before the map phase begins.
func (f *ConnectionFSM) OnIdentity(fn func(IdentityInfo)) *ConnectionFSM {
	f.onIdentity = fn
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
			fi := FailInfo{Err: err, Phase: PhaseLogin} // default phase
			var pe *phaseError
			if errors.As(err, &pe) {
				fi.Phase = pe.phase
				fi.Err = pe.err
			}
			f.onFailed(fi)
		}
	}
	return err
}

func (f *ConnectionFSM) stepTimeout() time.Duration {
	return f.server.StepTimeout
}

// connect is the internal implementation of Connect (no OnFailed dispatch).
// phaseError wraps an error with the AuthPhase that produced it.
// Used internally so Connect can construct FailInfo without passing phase through every call.
type phaseError struct {
	phase AuthPhase
	err   error
}

func (e *phaseError) Error() string { return e.err.Error() }
func (e *phaseError) Unwrap() error { return e.err }

func (f *ConnectionFSM) connect(ctx context.Context) error {
	// ── Phase 1: Login server ────────────────────────────────────────────────

	charServers, charIdx, err := f.runLoginPhase(ctx)
	if err != nil {
		return &phaseError{phase: PhaseLogin, err: err}
	}
	if charIdx < 0 || charIdx >= len(charServers) {
		return &phaseError{phase: PhaseLogin, err: fmt.Errorf("fsm: char server index %d out of range (have %d)", charIdx, len(charServers))}
	}

	chosen := charServers[charIdx]
	charAddr := fmt.Sprintf("%d.%d.%d.%d:%d",
		chosen.IP>>24, (chosen.IP>>16)&0xFF, (chosen.IP>>8)&0xFF, chosen.IP&0xFF,
		chosen.Port)

	// ── Phase 2: Char server ─────────────────────────────────────────────────

	mapAddr, err := f.runCharPhase(ctx, charAddr)
	if err != nil {
		return &phaseError{phase: PhaseChar, err: err}
	}

	// ── Phase 3: Map server ──────────────────────────────────────────────────

	if err := f.runMapPhase(ctx, mapAddr); err != nil {
		return &phaseError{phase: PhaseMap, err: err}
	}
	return nil
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

	pkt := fsmEncodeMasterLogin(f.server.Packetver, f.creds.Username, f.creds.Password)
	if err := writeDeadline(conn, pkt[:], f.stepTimeout()); err != nil {
		return nil, 0, fmt.Errorf("fsm: send login: %w", err)
	}

	// Determine which accept packet ID we expect.
	acceptID := uint16(0x0069)
	if f.server.Packetver >= 20170315 {
		acceptID = 0x0AC4
	}

	loginSess := NewLoginSession(f.server.Packetver)

	var charServers []CharServerInfo
	done := false
	var loginErr error

	loginSess.core.registerHandler(acceptID, func(data []byte, _ uint32) {
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
		loginSess.core.registerHandler(refuseID, func(data []byte, _ uint32) {
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

	loginSess.core.registerHandler(0x0081, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		if f.onServerNotify != nil {
			f.onServerNotify(NotifyInfo{Phase: PhaseLogin, Code: code})
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
	mapIP     uint32
	mapPort   uint16
	mapDomain string // from domain[128] in 0x0AC5 (pv >= 20170315); empty otherwise
	charID    uint32
	done      bool
	err       error
	mapName   string

	// char list accumulation
	rawChars      []byte // accumulated CHARACTER_INFO bytes
	charsExpected uint32 // total from 0x09A0; we send one CH_CHARLIST_REQ per character
	pagesRecv     uint32 // 0x099D responses received so far
	got09A0       bool   // whether we received HC_CHARLIST_NOTIFY
	gotCharList   bool   // whether char list is complete and OnCharList was called
	selectedSlot  uint8
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
	enterPkt := fsmEncodeGameLogin(f.accountID, f.sessionID1, f.sessionID2, f.sex)
	if err := writeDeadline(conn, enterPkt[:], f.stepTimeout()); err != nil {
		return "", fmt.Errorf("fsm: send char enter: %w", err)
	}

	// The char server immediately echoes back the account ID as a raw 4-byte uint32
	// (not a packet with a type header) before sending any framed packets.
	// Source: char_clif.cpp:851-853 — WFIFOL(fd,0) = account_id; WFIFOSET(fd,4)
	// Consume and discard these 4 bytes so the CharSession framer sees clean data.
	if err := conn.SetDeadline(time.Now().Add(f.stepTimeout())); err != nil {
		return "", fmt.Errorf("fsm: set char echo deadline: %w", err)
	}
	var echoAID [4]byte
	if _, err := io.ReadFull(conn, echoAID[:]); err != nil {
		return "", fmt.Errorf("fsm: read char account echo: %w", err)
	}

	charSess := NewCharSession(f.server.Packetver)

	// For PACKETVER < 20170315, 0x0081 is used as HC_NOTIFY_ZONESVR (28 bytes fixed)
	// AND as SC_NOTIFY_BAN (3 bytes). The char lengths table defaults to 3 (SC_NOTIFY_BAN).
	// Override to 28 so the framing engine consumes a full HC_NOTIFY_ZONESVR frame.
	// SC_NOTIFY_BAN arriving on the char connection during SelectingChar state will also
	// be framed as 28 bytes — the handler reads only byte[2] (result) and ignores the rest,
	// which is safe since SC_NOTIFY_BAN is always followed by connection close.
	// Source: common/packets.hpp PACKET_HC_NOTIFY_ZONESVR = 28 bytes at PACKETVER=20120000.
	if f.server.Packetver < 20170315 {
		charSess.core.lengths[0x0081] = 28
	}

	res := &charPhaseResult{}

	// 0x006C HC_REFUSE_ENTER — refused
	charSess.core.registerHandler(0x006C, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		res.err = fmt.Errorf("fsm: char server refused entry (code=%d)", code)
		res.done = true
	})

	// 0x0081 on char server — SC_NOTIFY_BAN or (for pv < 20170315) HC_NOTIFY_ZONESVR
	// Disambiguate by payload size: SC_NOTIFY_BAN = 3 bytes; HC_NOTIFY_ZONESVR = 28 bytes.
	charSess.core.registerHandler(0x0081, func(data []byte, _ uint32) {
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
			rawName := data[6:22]
			n := bytes.IndexByte(rawName, 0)
			if n < 0 {
				n = len(rawName)
			}
			res.mapName = strings.TrimSuffix(string(rawName[:n]), ".gat")
			res.done = true
			return
		}
		// SC_NOTIFY_BAN
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		if f.onServerNotify != nil {
			f.onServerNotify(NotifyInfo{Phase: PhaseChar, Code: code})
		}
		res.err = fmt.Errorf("fsm: char server notify ban (code=%d)", code)
		res.done = true
	})

	// 0x0AC5 HC_NOTIFY_ZONESVR (PACKETVER >= 20170315)
	// struct: int16 + uint32 CID + char mapname[16] + uint32 ip + uint16 port + char domain[128]
	// Source: common/packets.hpp at PACKETVER=20180307, size=156
	charSess.core.registerHandler(0x0AC5, func(data []byte, _ uint32) {
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
		rawName := data[6:22]
		n := bytes.IndexByte(rawName, 0)
		if n < 0 {
			n = len(rawName)
		}
		res.mapName = strings.TrimSuffix(string(rawName[:n]), ".gat")
		// domain[128] at bytes 28–155 (pv >= 20170315 only).
		// rAthena: common/packets.hpp:297 char domain[128]
		// OpenKore: XKoreProxy.pm:513 "mapUrl" — format "hostname" or "hostname:port"
		if len(data) >= 28+128 {
			rawDomain := data[28 : 28+128]
			nd := bytes.IndexByte(rawDomain, 0)
			if nd < 0 {
				nd = 128
			}
			if nd > 0 {
				res.mapDomain = string(rawDomain[:nd])
			}
		}
		res.done = true
	})

	// 0x082D HC_ACCEPT_ENTER2 (slot info, PACKETVER >= 20130000)
	// struct: int16 packetType + int16 packetLength + uint8 normal + uint8 premium +
	//         uint8 billing + uint8 producible + uint8 total + char extension[20]
	// Source: common/packets.hpp:508–517 PACKET_HC_ACCEPT_ENTER2
	// OpenKore: Receive.pm $charSvrSet{normal_slot}, {premium_slot}, {billing_slot}, etc.
	charSess.core.registerHandler(0x082D, func(data []byte, _ uint32) {
		if len(data) >= 9 && f.onSlotInfo != nil {
			f.onSlotInfo(SlotInfo{
				Normal:     data[4], // rAthena: normal
				Premium:    data[5], // rAthena: premium
				Billing:    data[6], // rAthena: billing
				Producible: data[7], // rAthena: producible
				Total:      data[8], // rAthena: total
			})
		}
		// Stay — char list arrives via 0x006B or paged via 0x09A0+0x099D
	})

	// 0x006B HC_ACCEPT_ENTER (char list)
	charSess.core.registerHandler(0x006B, func(data []byte, _ uint32) {
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
	var writeErr error
	charSess.core.registerHandler(0x09A0, func(data []byte, _ uint32) {
		if len(data) < 6 {
			return
		}
		total := binary.LittleEndian.Uint32(data[2:6])
		// total is the character count from 0x09A0; we send one CH_CHARLIST_REQ per
		// character and expect one 0x099D response per character (rAthena sends one
		// char per page in practice).
		res.charsExpected = total
		res.got09A0 = true

		// Send CH_CHARLIST_REQ (0x09A1) — one per character
		// struct: int16 packetType only (size=2)
		// Source: common/packets.hpp PACKET_CH_CHARLIST_REQ, size=2
		for i := uint32(0); i < total; i++ {
			pkt := fsmEncodeRequestCharacterPage()
			if err := writeDeadline(conn, pkt[:], f.stepTimeout()); err != nil {
				writeErr = err
				return
			}
		}
	})

	// 0x099D HC_ACK_CHARINFO_PER_PAGE: one page of char data
	// struct: int16 + int16 packetLength + CHARACTER_INFO characters[]
	// Header = 4 bytes.
	charSess.core.registerHandler(0x099D, func(data []byte, _ uint32) {
		if len(data) > 4 {
			res.rawChars = append(res.rawChars, data[4:]...)
		}
		res.pagesRecv++
		if res.got09A0 && res.pagesRecv >= res.charsExpected {
			slot := f.pickCharSlot(res.rawChars)
			res.selectedSlot = slot
			res.gotCharList = true
		}
	})

	// Inner feed loop: runs until charList ready, then sends 0x0066, then waits for zone info.
	slotSent := false

	for !res.done {
		if writeErr != nil {
			return "", fmt.Errorf("fsm: send charlist req: %w", writeErr)
		}
		if res.gotCharList && !slotSent {
			// Send 0x0066 CH_SELECT_CHAR
			selPkt := fsmEncodeCharLogin(res.selectedSlot)
			if err := writeDeadline(conn, selPkt[:], f.stepTimeout()); err != nil {
				return "", fmt.Errorf("fsm: send select char: %w", err)
			}
			slotSent = true
		}

		if err := f.feedStep(conn, charSess, f.stepTimeout()); err != nil {
			return "", fmt.Errorf("fsm: char feed: %w", err)
		}
	}

	if writeErr != nil {
		return "", fmt.Errorf("fsm: send charlist req: %w", writeErr)
	}

	if res.err != nil {
		return "", res.err
	}

	mapAddr := fmt.Sprintf("%d.%d.%d.%d:%d",
		res.mapIP>>24, (res.mapIP>>16)&0xFF, res.mapIP>>8&0xFF, res.mapIP&0xFF,
		res.mapPort)

	if f.onIdentity != nil {
		f.onIdentity(IdentityInfo{
			AccountID:    f.accountID,
			CharID:       f.charID,
			SelectedSlot: res.selectedSlot,
			Sex:          f.sex,
			MapName:      res.mapName,
			MapIP:        res.mapIP,
			MapPort:      res.mapPort,
			MapDomain:    res.mapDomain,
		})
	}

	return mapAddr, nil
}

// pickCharSlot decodes rawChars into []events.CharacterInfoEntry, calls OnCharList
// if registered, else returns Credentials.CharSlot.
func (f *ConnectionFSM) pickCharSlot(rawChars []byte) uint8 {
	if f.onCharList != nil {
		entries := decode.DecodeCharacterInfoList(rawChars, f.server.Packetver)
		return f.onCharList(entries)
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
	connTransferred := false
	defer func() {
		if !connTransferred {
			conn.Close()
		}
	}()

	mapSess := NewMapSession(f.server.Packetver)

	// Enable C→S obfuscation if keys exist for this packetver.
	k0, k1, k2 := obfuscationKeysFor(f.server.Packetver)
	if k0|k1|k2 != 0 {
		mapSess.enableObfuscation(k0, k1, k2)
	}

	// Send 0x0436 CZ_ENTER / CZ_ENTER2
	// Source: clif_shuffle.hpp > 20180307 block; length depends on PACKETVER_RE_NUM >= 20211103.
	enterPkt := fsmEncodeMapLogin(f.accountID, f.charID, f.sessionID1, f.sex, f.server.Packetver)
	fsmEncodePacketID(mapSess, enterPkt)
	if err := writeDeadline(conn, enterPkt, f.stepTimeout()); err != nil {
		return fmt.Errorf("fsm: send map enter: %w", err)
	}

	type mapResult struct {
		done      bool
		err       error
		x         uint16
		y         uint16
		dir       uint8
		startTime uint32
		font      uint16
		sex       uint8
	}
	res := &mapResult{}

	// 0x0283 ZC_AID: account ID echo (PACKETVER >= 20070521)
	// struct: int16 + uint32 AID — size=6
	// Source: clif.cpp:10731 WFIFOW(fd,0)=0x283; WFIFOL(fd,2)=sd->id
	mapSess.core.registerHandler(0x0283, func(data []byte, _ uint32) {
		// Store if needed; currently we already have accountID from login.
	})

	// 0x0074 ZC_REFUSE_ENTER
	mapSess.core.registerHandler(0x0074, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		res.err = fmt.Errorf("fsm: map server refused entry (code=%d)", code)
		res.done = true
	})

	// 0x0081 SC_NOTIFY_BAN (on map server)
	mapSess.core.registerHandler(0x0081, func(data []byte, _ uint32) {
		code := byte(0)
		if len(data) >= 3 {
			code = data[2]
		}
		if f.onServerNotify != nil {
			f.onServerNotify(NotifyInfo{Phase: PhaseMap, Code: code})
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
		loadedPkt := fsmEncodeMapLoaded()
		fsmEncodePacketID(mapSess, loadedPkt[:])
		if err := writeDeadline(conn, loadedPkt[:], f.stepTimeout()); err != nil {
			res.err = fmt.Errorf("fsm: send map loaded: %w", err)
			res.done = true
			return
		}

		// Send 0x007E or 0x0360 CZ_REQUEST_TIME (tick sync)
		// struct: int16 + uint32 clientTick = 6 bytes; Source: clif.cpp:11196-11197
		tickArr := fsmEncodeTimeSyncResponse(f.server.Packetver)
		tickPkt := tickArr[:]
		fsmEncodePacketID(mapSess, tickPkt)
		if err := writeDeadline(conn, tickPkt, f.stepTimeout()); err != nil {
			res.err = fmt.Errorf("fsm: send tick sync: %w", err)
			res.done = true
			return
		}

		if len(data) >= 6 {
			res.startTime = binary.LittleEndian.Uint32(data[2:6])
		}
		if len(data) >= 9 {
			x, y, dir := packing.DecodePosDir(data[6:9])
			res.x = x
			res.y = y
			res.dir = dir
		}
		if len(data) >= 13 {
			res.font = binary.LittleEndian.Uint16(data[11:13])
		}
		if len(data) >= 14 {
			res.sex = data[13]
		}
		res.done = true
	}

	// Register the appropriate ZC_ACCEPT_ENTER packet ID based on packetver.
	// Source: src/map/packets.hpp:545-575
	//   PACKETVER < 20080102                                    → 0x0073  ZC_ACCEPT_ENTER  (11 bytes)
	//   PACKETVER < 20141022 || PACKETVER >= 20160330           → 0x02EB  ZC_ACCEPT_ENTER2 (13 bytes)
	//   else (>= 20141022 && < 20160330)                        → 0x0A18  ZC_ACCEPT_ENTER3 (14 bytes)
	mapSess.core.registerHandler(zcAcceptEnterID(f.server.Packetver), onMapEnter)

	// Fire OnMapSessionCreated after FSM auth handlers are registered but before
	// feedUntil processes any bytes. This is the correct place for callers to
	// register receive-direction semantic handlers that must capture packets
	// co-delivered with ZC_ACCEPT_ENTER (e.g., the inventory burst sent by
	// clif_parse_LoadEndAck in response to 0x007D CZ_NOTIFY_ACTORINIT).
	// See worklog 0068 for the full root-cause analysis.
	if f.onMapSessionCreated != nil {
		f.onMapSessionCreated(mapSess)
	}

	if err := f.feedUntil(conn, mapSess, f.stepTimeout(), &res.done); err != nil {
		return fmt.Errorf("fsm: map feed: %w", err)
	}
	if res.err != nil {
		return res.err
	}

	// Hand off the conn and mapSess to goKore via OnReady.
	// After this call the FSM releases conn — goKore owns it.
	// connTransferred is set to true before calling onReady so that the deferred
	// conn.Close() does not fire on the normal hand-off path.
	// NOTE: if onReady panics, connTransferred is already true and the defer will
	// NOT close conn. This is intentional: the caller (goKore) is responsible for
	// recovering from its own callback panics and closing the connection.
	if f.onReady != nil {
		connTransferred = true
		f.onReady(mapSess, conn, ReadyInfo{
			X:         res.x,
			Y:         res.y,
			Dir:       res.dir,
			StartTime: res.startTime,
			Font:      res.font,
			Sex:       res.sex,
		})
	}
	return nil
}

// ── I/O helpers ──────────────────────────────────────────────────────────────

// fsmEncodePacketID applies C→S obfuscation to the packet ID field (bytes 0-1) of
// pkt in-place. If obfuscation is not enabled on s, this is a no-op.
// pkt must be at least 2 bytes.
func fsmEncodePacketID(s *MapSession, pkt []byte) {
	id := binary.LittleEndian.Uint16(pkt[0:2])
	s.encodePacketID(&id)
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
			var unk ErrUnknownPacket
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

// ── Inline auth-phase packet encoders ─────────────────────────────────────────
//
// These are local copies of generated encode functions needed by the FSM.
// They cannot call pkg/encode because pkg/encode imports pkg/session (for
// RegisterSendEncoder), which would create an import cycle.
// The implementations are identical to the generated versions in pkg/encode.

// fsmEncodeMasterLogin encodes 0x0064 CA_LOGIN (55 bytes).
// Source: common/packets.hpp PACKET_CA_LOGIN; GCC-verified = 55 bytes.
func fsmEncodeMasterLogin(packetver uint32, username, password string) [55]byte {
	var p [55]byte
	p[0] = 0x64
	p[1] = 0x00
	binary.LittleEndian.PutUint32(p[2:], packetver) // rAthena: version
	copy(p[6:30], username)                         // rAthena: username
	copy(p[30:54], password)                        // rAthena: password
	// p[54] clienttype = 0x00
	return p
}

// fsmEncodeGameLogin encodes 0x0065 CH_ENTER (17 bytes).
// Source: char_clif.cpp:820; GCC-verified = 17 bytes.
func fsmEncodeGameLogin(aid, authCode, loginID2 uint32, sex uint8) [17]byte {
	var p [17]byte
	p[0] = 0x65
	p[1] = 0x00
	binary.LittleEndian.PutUint32(p[2:], aid)       // rAthena: AID
	binary.LittleEndian.PutUint32(p[6:], authCode)  // rAthena: AuthCode
	binary.LittleEndian.PutUint32(p[10:], loginID2) // rAthena: login_id2
	// p[14:16] clienttype = 0
	p[16] = sex // rAthena: sex
	return p
}

// fsmEncodeCharLogin encodes 0x0066 CH_SELECT_CHAR (3 bytes).
// Source: common/packets.hpp PACKET_CH_SELECT_CHAR; GCC-verified = 3 bytes.
func fsmEncodeCharLogin(slot uint8) [3]byte {
	var p [3]byte
	p[0] = 0x66
	p[1] = 0x00
	p[2] = slot // rAthena: slot
	return p
}

// fsmEncodeRequestCharacterPage encodes 0x09A1 CH_CHARLIST_REQ (2 bytes).
// Source: common/packets.hpp PACKET_CH_CHARLIST_REQ; GCC-verified = 2 bytes.
func fsmEncodeRequestCharacterPage() [2]byte {
	var p [2]byte
	p[0] = 0xa1
	p[1] = 0x09
	return p
}

// zcAcceptEnterID returns the packet ID rAthena uses for ZC_ACCEPT_ENTER at
// the given packetver.
//
// Source: src/map/packets.hpp:545-575
//
//	PACKETVER < 20080102                          → 0x0073  ZC_ACCEPT_ENTER  (11 bytes)
//	PACKETVER < 20141022 || PACKETVER >= 20160330 → 0x02EB  ZC_ACCEPT_ENTER2 (13 bytes)
//	else (>= 20141022 && < 20160330)              → 0x0A18  ZC_ACCEPT_ENTER3 (14 bytes)
func zcAcceptEnterID(packetver uint32) uint16 {
	switch {
	case packetver >= 20141022 && packetver < 20160330:
		return 0x0A18
	case packetver >= 20080102:
		return 0x02EB
	default:
		return 0x0073
	}
}

// fsmEncodeMapLogin encodes 0x0436 CZ_ENTER / CZ_ENTER2.
//
// The wire length depends on packetver:
//
//   - 19 bytes (sex at offset 18) in all other cases:
//     id(2) + AID(4) + GID(4) + AuthCode(4) + clientTime(4) + sex(1)
//     Source: clif_shuffle.hpp:4747
//     parseable_packet( 0x0436, 19, clif_parse_WantToConnection, 2, 6, 10, 14, 18 )
//
//   - 23 bytes (sex at offset 22) when:
//     PACKETVER_RE_NUM >= 20211103  → packetver in [20211103, 20211118]
//     PACKETVER_MAIN_NUM >= 20220330 → packetver >= 20220330 (outside RE window)
//     id(2) + AID(4) + GID(4) + AuthCode(4) + clientTime(4) + tick(4) + sex(1)
//     Source: clif_shuffle.hpp:4744-4745
//     #if PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330
//     parseable_packet( 0x0436, 23, clif_parse_WantToConnection, 2, 6, 10, 14, 22 )
//
// rAthena config/packets.hpp line 22: PACKETVER_RE is defined (→ PACKETVER_RE_NUM=PACKETVER) when
// (PACKETVER > 20151104 && PACKETVER < 20180704) || (PACKETVER >= 20200902 && PACKETVER <= 20211118).
// For PACKETVER outside that range, PACKETVER_MAIN_NUM=PACKETVER.
// GCC-verified boundaries: 20211103→23B, 20211118→23B, 20211119→19B, 20220329→19B, 20220330→23B.
func fsmEncodeMapLogin(aid, gid, authCode uint32, sex uint8, packetver uint32) []byte {
	// 23-byte variant: PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330
	// Source: clif_shuffle.hpp:4744-4745
	if (packetver >= 20211103 && packetver <= 20211118) || packetver >= 20220330 {
		p := make([]byte, 23)
		p[0] = 0x36
		p[1] = 0x04
		binary.LittleEndian.PutUint32(p[2:], aid)       // rAthena: AID        (pos[0]=2)
		binary.LittleEndian.PutUint32(p[6:], gid)       // rAthena: GID        (pos[1]=6)
		binary.LittleEndian.PutUint32(p[10:], authCode) // rAthena: AuthCode   (pos[2]=10)
		// p[14:18] clientTime = 0                       // rAthena: clientTick (pos[3]=14)
		// p[18:22] tick = 0                             // extra field
		p[22] = sex // rAthena: sex        (pos[4]=22)
		return p
	}
	// 19-byte variant: default
	// Source: clif_shuffle.hpp:4747
	p := make([]byte, 19)
	p[0] = 0x36
	p[1] = 0x04
	binary.LittleEndian.PutUint32(p[2:], aid)       // rAthena: AID        (pos[0]=2)
	binary.LittleEndian.PutUint32(p[6:], gid)       // rAthena: GID        (pos[1]=6)
	binary.LittleEndian.PutUint32(p[10:], authCode) // rAthena: AuthCode   (pos[2]=10)
	// p[14:18] clientTime = 0                       // rAthena: clientTick (pos[3]=14)
	p[18] = sex // rAthena: sex        (pos[4]=18)
	return p
}

// fsmEncodeMapLoaded encodes 0x007D CZ_NOTIFY_ACTORINIT (2 bytes).
// Source: clif.cpp:10742; GCC-verified = 2 bytes.
func fsmEncodeMapLoaded() [2]byte {
	var p [2]byte
	p[0] = 0x7d
	p[1] = 0x00
	return p
}

// fsmEncodeTimeSyncResponse encodes CZ_REQUEST_TIME:
//   - 0x007E (pv < 20101124): int16 + uint32 = 6 bytes
//   - 0x0360 (pv >= 20101124): int16 + uint32 = 6 bytes
//
// Source: clif.cpp:11196-11197.
func fsmEncodeTimeSyncResponse(packetver uint32) [6]byte {
	var p [6]byte
	if packetver >= 20101124 {
		p[0] = 0x60
		p[1] = 0x03
	} else {
		p[0] = 0x7e
		p[1] = 0x00
	}
	// clientTick = 0 at bytes [2:6]
	return p
}
