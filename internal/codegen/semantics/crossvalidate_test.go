// Cross-validation tests: verify that packet_id values in semantics/mappings.yaml
// match authoritative packet ID declarations in rAthena source headers.
//
// These tests use the real rAthena source tree and are tagged "integration"
// so they run on developer machines (where ~/personal/rathena exists) but
// are skipped in environments without the source tree.
//
// Run with:
//
//	go test -tags integration ./internal/codegen/semantics/ -run TestSemanticDB
//
// The four categories of implementations:
//
//  1. DEFINE_PACKET_HEADER / DEFINE_PACKET_ID structs (~82% of DB):
//     authoritative binding is in packets_struct.hpp / packets.hpp / common/packets.hpp.
//     We parse all DEFINE_PACKET_HEADER and DEFINE_PACKET_ID entries and verify.
//
//  2. packet_headers enum structs (~4% of DB):
//     old-style structs (packet_idle_unit, packet_unit_walking, etc.) get their
//     IDs from the PACKETVER-conditional packet_headers enum in packets_struct.hpp.
//     We parse ALL branches of the enum and accept any ID ever assigned to the type.
//
//  3. SYNTH_* synthetic structs (~12% of DB):
//     hand-crafted, no rAthena struct. No automated verification possible.
//     Logged for human review; do not fail.
//
//  4. Truly not-found structs (~2% of DB):
//     structs that exist but use neither DEFINE_PACKET_HEADER nor the enum.
//     Verified manually via clif.cpp comments; added to KnownCorrectBindings.
//
//go:build integration

package semantics_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

// rathenaHeaders lists all rAthena header files that contain DEFINE_PACKET_HEADER
// or DEFINE_PACKET_ID declarations. Relative to rathenaRoot/src/.
var rathenaHeaders = []string{
	"map/packets_struct.hpp",
	"map/packets.hpp",
	"common/packets.hpp",
	"char/packets_hc.hpp",
	"login/packets_la.hpp",
}

// definePacketHeaderRe matches both:
//   - DEFINE_PACKET_HEADER(StructSuffix, 0xNNNN)
//   - DEFINE_PACKET_ID(StructSuffix, 0xNNNN)
var definePacketHeaderRe = regexp.MustCompile(
	`DEFINE_PACKET_(?:HEADER|ID)\s*\(\s*(\w+)\s*,\s*(0x[0-9a-fA-F]+|\d+)\s*\)`)

// packetHeadersEnumEntryRe matches enum entries like:
//
//	idle_unitType = 0x9ff,
//	spawn_unitType = 0x9fe,
var packetHeadersEnumEntryRe = regexp.MustCompile(`\b(\w+Type)\s*=\s*(0x[0-9a-fA-F]+)`)

// normID normalises a packet ID string to lowercase "0xNNNN".
func normID(s string) string {
	s = strings.TrimSpace(s)
	var num uint64
	fmt.Sscanf(s, "%v", &num)
	return fmt.Sprintf("0x%04x", num)
}

// parsePacketHeadersEnum reads the packet_headers enum block from packets_struct.hpp
// and returns a map of lookup key → set of all packet IDs ever assigned across ALL
// PACKETVER branches (since any historical ID may be in the DB).
//
// The lookup key convention:
//
//	idle_unitType  → "PACKET_IDLE_UNIT"   (matches structSuffix("packet_idle_unit"))
//	spawn_unitType → "PACKET_SPAWN_UNIT"
func parsePacketHeadersEnum(packetsStructPath string) (map[string]map[string]struct{}, error) {
	data, err := os.ReadFile(packetsStructPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", packetsStructPath, err)
	}
	src := string(data)

	enumStart := strings.Index(src, "enum packet_headers {")
	if enumStart < 0 {
		return nil, fmt.Errorf("packet_headers enum not found")
	}
	enumEnd := strings.Index(src[enumStart:], "};")
	if enumEnd < 0 {
		return nil, fmt.Errorf("packet_headers enum closing }; not found")
	}
	enumBody := src[enumStart : enumStart+enumEnd+2]

	result := make(map[string]map[string]struct{})
	for _, m := range packetHeadersEnumEntryRe.FindAllStringSubmatch(enumBody, -1) {
		enumName := m[1]                                 // e.g. "idle_unitType"
		pid := normID(m[2])                              // e.g. "0x09ff"
		baseName := strings.TrimSuffix(enumName, "Type") // e.g. "idle_unit"
		key := "PACKET_" + strings.ToUpper(baseName)     // e.g. "PACKET_IDLE_UNIT"
		if result[key] == nil {
			result[key] = make(map[string]struct{})
		}
		result[key][pid] = struct{}{}
	}
	return result, nil
}

// parseAllDefines reads all DEFINE_PACKET_HEADER, DEFINE_PACKET_ID, and
// packet_headers enum entries from rAthena source headers.
// Returns map of uppercase struct key → set of valid packet IDs.
// IDs are normalised to lowercase "0xNNNN".
//
// Key conventions:
//   - DEFINE_PACKET_HEADER/ID: strip "PACKET_" prefix → e.g. "ZC_NOTIFY_CHAT"
//   - packet_headers enum: "PACKET_" + uppercase base → e.g. "PACKET_IDLE_UNIT"
func parseAllDefines(rathenaRoot string) (map[string]map[string]struct{}, error) {
	result := make(map[string]map[string]struct{})

	// 1. DEFINE_PACKET_HEADER / DEFINE_PACKET_ID from all header files.
	for _, rel := range rathenaHeaders {
		path := filepath.Join(rathenaRoot, "src", rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		for _, m := range definePacketHeaderRe.FindAllSubmatch(data, -1) {
			suffix := strings.ToUpper(string(m[1]))
			pid := normID(string(m[2]))
			if result[suffix] == nil {
				result[suffix] = make(map[string]struct{})
			}
			result[suffix][pid] = struct{}{}
		}
	}

	// 2. packet_headers enum from packets_struct.hpp.
	enumPath := filepath.Join(rathenaRoot, "src", "map", "packets_struct.hpp")
	enumBindings, err := parsePacketHeadersEnum(enumPath)
	if err != nil {
		// Non-fatal — log but continue; struct-based verification still works.
		fmt.Printf("WARNING: parsePacketHeadersEnum: %v\n", err)
	} else {
		for key, ids := range enumBindings {
			if result[key] == nil {
				result[key] = make(map[string]struct{})
			}
			for pid := range ids {
				result[key][pid] = struct{}{}
			}
		}
	}

	return result, nil
}

// structSuffix returns the lookup key for a struct name in the defines map.
//
//	"PACKET_ZC_NOTIFY_CHAT"  → "ZC_NOTIFY_CHAT"       (DEFINE_PACKET_HEADER key)
//	"PACKET_AC_ACCEPT_LOGIN" → "AC_ACCEPT_LOGIN"
//	"ZC_NOTIFY_CHAT"         → "ZC_NOTIFY_CHAT"        (already a suffix)
//	"packet_idle_unit"       → "PACKET_IDLE_UNIT"      (packet_headers enum key)
//	"packet_unit_walking"    → "PACKET_UNIT_WALKING"
//	"SYNTH_*"                → ""                      (synthetic, skip)
//
// IMPORTANT: check lowercase "packet_" prefix BEFORE uppercasing, because
// "packet_idle_unit".ToUpper() = "PACKET_IDLE_UNIT" which would be caught by
// the PACKET_ strip branch instead of the enum key branch.
func structSuffix(name string) string {
	if strings.HasPrefix(name, "packet_") {
		// Old-style struct: packet_idle_unit → "PACKET_IDLE_UNIT"
		// to match the packet_headers enum key convention in parseAllDefines.
		return strings.ToUpper(name) // "PACKET_IDLE_UNIT"
	}
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "PACKET_") {
		return strings.TrimPrefix(upper, "PACKET_") // "ZC_NOTIFY_CHAT"
	}
	if strings.HasPrefix(upper, "SYNTH_") {
		return "" // unverifiable synthetic
	}
	return upper
}

// TestSemanticDB_PacketIDMatchesDefinePacketHeader verifies that for every
// implementation in semantics/mappings.yaml whose struct can be found in rAthena
// source (via DEFINE_PACKET_HEADER, DEFINE_PACKET_ID, or packet_headers enum),
// the claimed packet_id matches one of the valid IDs for that struct.
//
// Covers ~86% of all DB implementations. Remaining ~14% are SYNTH_* or structs
// with no automated binding (logged as informational).
func TestSemanticDB_PacketIDMatchesDefinePacketHeader(t *testing.T) {
	home := os.Getenv("HOME")
	rathenaRoot := filepath.Join(home, "personal", "rathena")
	if _, err := os.Stat(rathenaRoot); os.IsNotExist(err) {
		t.Skipf("rAthena source not found at %s — skipping integration test", rathenaRoot)
	}

	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	defines, err := parseAllDefines(rathenaRoot)
	if err != nil {
		t.Fatalf("parseAllDefines: %v", err)
	}
	t.Logf("Defines loaded: %d unique struct keys (DEFINE_PACKET_HEADER/ID + enum)",
		len(defines))

	type result struct {
		action     string
		packetID   string
		structName string
		category   string
		validIDs   []string
	}

	var verified, mismatches, synths, notFound []result

	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			pid := normID(impl.PacketID)
			suffix := structSuffix(impl.StructName)
			r := result{action: name, packetID: pid, structName: impl.StructName}

			switch {
			case strings.HasPrefix(impl.StructName, "SYNTH_"):
				r.category = "synth"
				synths = append(synths, r)

			case suffix == "":
				r.category = "not_found"
				notFound = append(notFound, r)

			default:
				validMap, ok := defines[suffix]
				if !ok {
					r.category = "not_found"
					notFound = append(notFound, r)
				} else if _, match := validMap[pid]; match {
					r.category = "verified"
					verified = append(verified, r)
				} else {
					r.category = "mismatch"
					for id := range validMap {
						r.validIDs = append(r.validIDs, id)
					}
					mismatches = append(mismatches, r)
				}
			}
		}
	}

	t.Logf("Results: verified=%d mismatches=%d synth=%d not_found=%d",
		len(verified), len(mismatches), len(synths), len(notFound))

	if len(synths) > 0 {
		t.Logf("SYNTH_* (unverifiable by design): %d implementations", len(synths))
	}
	if len(notFound) > 0 {
		t.Logf("Struct not found in any rAthena source (%d) — add to KnownCorrectBindings if verified:",
			len(notFound))
		for _, r := range notFound {
			t.Logf("  %s: %s → %s", r.action, r.packetID, r.structName)
		}
	}

	// MISMATCHES are hard failures.
	for _, r := range mismatches {
		t.Errorf("MISMATCH: action=%q claims packet_id=%s for struct %s\n"+
			"  rAthena source says valid IDs for %s are: %v\n"+
			"  Fix: update semantics DB via MCP",
			r.action, r.packetID, r.structName, r.structName, r.validIDs)
	}
}

// TestSemanticDB_NoDuplicatePacketIDs verifies no two different actions
// claim the same packet_id. A packet ID can only map to one semantic action.
func TestSemanticDB_NoDuplicatePacketIDs(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	seen := make(map[string]string) // pid → action_name
	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			pid := normID(impl.PacketID)
			if prev, exists := seen[pid]; exists {
				t.Errorf("duplicate packet_id %s: claimed by both %q and %q",
					pid, prev, name)
			} else {
				seen[pid] = name
			}
		}
	}
	t.Logf("checked %d unique packet IDs across all actions", len(seen))
}

// TestSemanticDB_PacketverRangesSane verifies YYYYMMDD date sanity for all
// packetver_min / packetver_max values.
func TestSemanticDB_PacketverRangesSane(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	const minSane = 20030000
	const maxSane = 20991231

	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			min, max := impl.PacketverMin, impl.PacketverMax
			if min != 0 && (min < minSane || min > maxSane) {
				t.Errorf("%s impl %s: packetver_min=%d outside [%d, %d]",
					name, impl.PacketID, min, minSane, maxSane)
			}
			if max != 0 && (max < minSane || max > maxSane) {
				t.Errorf("%s impl %s: packetver_max=%d outside [%d, %d]",
					name, impl.PacketID, max, minSane, maxSane)
			}
			if min != 0 && max != 0 && min >= max {
				t.Errorf("%s impl %s: packetver_min=%d >= packetver_max=%d",
					name, impl.PacketID, min, max)
			}
		}
	}
}

// TestSemanticDB_KnownCorrectBindings is a regression whitelist of manually verified
// bindings. Adding an entry documents the source verification and prevents future
// incorrect DB edits from going undetected.
//
// Does NOT require rAthena source — pure DB consistency check.
func TestSemanticDB_KnownCorrectBindings(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	type binding struct {
		action     string
		packetID   string // normalised "0xNNNN"
		structName string
		note       string // source of verification
	}

	verified := []binding{
		// ── DEFINE_PACKET_HEADER structs ──────────────────────────────────────
		{"actor_died_or_disappeared", "0x0080", "PACKET_ZC_NOTIFY_VANISH",
			"DEFINE_PACKET_HEADER(ZC_NOTIFY_VANISH, 0x80)"},
		{"item_pickup", "0x00a0", "PACKET_ZC_ITEM_PICKUP_ACK",
			"DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x00a0)"},
		{"ac_accept_login", "0x0069", "PACKET_AC_ACCEPT_LOGIN",
			"common/packets.hpp DEFINE_PACKET_HEADER(AC_ACCEPT_LOGIN, 0x69)"},
		{"ac_accept_login", "0x0ac4", "PACKET_AC_ACCEPT_LOGIN",
			"common/packets.hpp DEFINE_PACKET_HEADER(AC_ACCEPT_LOGIN, 0xac4)"},
		{"chat_message", "0x008d", "PACKET_ZC_NOTIFY_CHAT",
			"packets.hpp DEFINE_PACKET_HEADER(ZC_NOTIFY_CHAT, 0x8d) + clif.cpp:6753 comment"},
		{"market_purchase", "0x09d6", "PACKET_CZ_NPC_MARKET_PURCHASE",
			"DEFINE_PACKET_HEADER(CZ_NPC_MARKET_PURCHASE, 0x09d6)"},
		{"zc_ack_ranking", "0x097d", "PACKET_ZC_ACK_RANKING",
			"DEFINE_PACKET_HEADER(ZC_ACK_RANKING, 0x97d) — 0x0af6 is ZC_ACK_RANKING2"},

		// ── packet_headers enum (old-style structs) ───────────────────────────
		{"actor_exists", "0x0078", "packet_idle_unit",
			"packet_headers enum: idle_unitType = 0x78 (PACKETVER < 4)"},
		{"actor_exists", "0x09ff", "packet_idle_unit",
			"packet_headers enum: idle_unitType = 0x9ff (PACKETVER >= 20150513)"},
		{"actor_moved", "0x007b", "packet_unit_walking",
			"packet_headers enum: unit_walkingType = 0x7b (PACKETVER < 4)"},
		{"actor_moved", "0x09fd", "packet_unit_walking",
			"packet_headers enum: unit_walkingType = 0x9fd (PACKETVER >= 20150513)"},
		{"actor_connected", "0x0079", "packet_spawn_unit",
			"packet_headers enum: spawn_unitType = 0x79 (PACKETVER < 4)"},
		{"actor_connected", "0x09fe", "packet_spawn_unit",
			"packet_headers enum: spawn_unitType = 0x9fe (PACKETVER >= 20150513)"},
		{"monster_hp_update", "0x0977", "packet_monster_hp",
			"clif_packetdb.hpp: packet(0x0977,14); //Monster HP Bar"},

		// ── SYNTH_* verified manually ─────────────────────────────────────────
		{"account_id", "0x0283", "SYNTH_ZC_AID",
			"synthetic: clif.cpp sends 0x0283 with just the account ID"},

		// ── No DEFINE, verified via clif.cpp source comment ──────────────────
		{"self_chat", "0x008e", "PACKET_ZC_NOTIFY_PLAYERCHAT",
			"clif.cpp:6661: /// 008e <packet len>.W <message>.?B (ZC_NOTIFY_PLAYERCHAT)"},
		{"enter_world", "0x007d", "PACKET_CZ_NOTIFY_ACTORINIT",
			"clif.cpp:10743: /// 007d (CZ_NOTIFY_ACTORINIT) + clif_packetdb.hpp:32"},
		{"public_chat", "0x008c", "PACKET_CZ_REQUEST_CHAT",
			"clif_packetdb.hpp: parseable_packet(0x008c,-1,clif_parse_GlobalMessage,2,4)"},
		{"monster_hp_update", "0x0977", "packet_monster_hp",
			"clif_packetdb.hpp: packet(0x0977,14); //Monster HP Bar"},
		{"deal_finalize", "0x00eb", "PACKET_CZ_CONCLUDE_EXCHANGE_ITEM",
			"clif_packetdb.hpp: parseable_packet(0x00eb,2,clif_parse_TradeOk,0)"},
		{"reply_party_invite", "0x00ff", "PACKET_CZ_REPLY_JOIN_GROUP",
			"clif_packetdb.hpp: parseable_packet(0x00ff,...)"},
		{"shop_buy", "0x00c8", "PACKET_CZ_PC_PURCHASE_ITEMLIST",
			"clif_packetdb.hpp: parseable_packet(0x00c8,-1,clif_parse_NpcBuyListSend,2,4)"},
		// game_login 0x0275: tRO-specific extension (not in standard kRO rAthena)
		// standard kRO uses 0x0065; 0x0275 is OpenKore Network/Send/tRO.pm: game_login 0275
		{"game_login", "0x0275", "PACKET_CH_ENTER",
			"tRO-specific extension (OpenKore Send/tRO.pm: game_login 0275); not in kRO rAthena"},
	}

	for _, b := range verified {
		action, ok := db.Actions[b.action]
		if !ok {
			t.Errorf("MISSING action %q (note: %s)", b.action, b.note)
			continue
		}
		found := false
		for _, impl := range action.Implementations {
			if normID(impl.PacketID) == b.packetID && impl.StructName == b.structName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("action %q: missing verified binding packet_id=%s struct=%s\n  note: %s\n  actual: %v",
				b.action, b.packetID, b.structName, b.note, action.Implementations)
		}
	}
}
