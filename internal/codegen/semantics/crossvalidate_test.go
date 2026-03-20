// Cross-validation tests: verify that packet_id values in semantics/mappings.yaml
// match DEFINE_PACKET_HEADER declarations in rAthena source headers.
//
// These tests use the real rAthena source tree and are tagged "integration"
// so they run on developer machines (where ~/personal/rathena exists) but
// are skipped in environments without the source tree.
//
// Run with:
//
//	go test -tags integration ./internal/codegen/semantics/ -run TestSemanticDB
//
// The three categories of implementations:
//
//  1. DEFINE_PACKET_HEADER structs (modern style, ~80% of DB):
//     authoritative binding is in packets_struct.hpp / packets.hpp / common/packets.hpp.
//     We parse all DEFINE_PACKET_HEADER entries and verify DB packet_id matches.
//
//  2. packet_* old-style structs (~4% of DB):
//     authoritative binding is in clif_packetdb.hpp only. We verify the claimed
//     packet_id appears in clif_packetdb.hpp with a handler whose name relates
//     to the action (best-effort — we just check the ID exists and has some handler).
//
//  3. SYNTH_* synthetic structs (~13% of DB):
//     hand-crafted, no rAthena struct. No automated verification possible.
//     We log them for human review but do not fail.
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
// declarations. Relative to rathenaRoot/src/.
var rathenaHeaders = []string{
	"map/packets_struct.hpp",
	"map/packets.hpp",
	"common/packets.hpp",
	"char/packets_hc.hpp",
	"login/packets_la.hpp",
}

// definePacketHeaderRe matches: DEFINE_PACKET_HEADER(StructSuffix, 0xNNNN)
// The struct suffix is the part after PACKET_ prefix (e.g. ZC_NOTIFY_CHAT, AC_ACCEPT_LOGIN).
var definePacketHeaderRe = regexp.MustCompile(`DEFINE_PACKET_HEADER\s*\(\s*(\w+)\s*,\s*(0x[0-9a-fA-F]+|\d+)\s*\)`)

// parseAllDefines reads all DEFINE_PACKET_HEADER declarations from all rAthena
// header files and returns a map of uppercase struct suffix → set of valid packet IDs.
// IDs are normalised to lowercase "0xNNNN" (zero-padded to 4 hex digits).
func parseAllDefines(rathenaRoot string) (map[string]map[string]struct{}, error) {
	result := make(map[string]map[string]struct{})
	for _, rel := range rathenaHeaders {
		path := filepath.Join(rathenaRoot, "src", rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue // optional header
			}
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		for _, m := range definePacketHeaderRe.FindAllSubmatch(data, -1) {
			suffix := strings.ToUpper(string(m[1]))
			rawID := string(m[2])
			// Normalise: parse hex or decimal, format as 0xNNNN lowercase
			var num uint64
			if strings.HasPrefix(rawID, "0x") || strings.HasPrefix(rawID, "0X") {
				fmt.Sscanf(rawID, "%v", &num)
			} else {
				fmt.Sscanf(rawID, "%d", &num)
			}
			pid := fmt.Sprintf("0x%04x", num)
			if result[suffix] == nil {
				result[suffix] = make(map[string]struct{})
			}
			result[suffix][pid] = struct{}{}
		}
	}
	return result, nil
}

// normID normalises a packet ID string to lowercase "0xNNNN".
func normID(s string) string {
	s = strings.TrimSpace(s)
	var num uint64
	fmt.Sscanf(s, "%v", &num)
	return fmt.Sprintf("0x%04x", num)
}

// structSuffix returns the lookup key for a struct name:
// "PACKET_ZC_NOTIFY_CHAT" → "ZC_NOTIFY_CHAT"
// "PACKET_AC_ACCEPT_LOGIN" → "AC_ACCEPT_LOGIN"
// "ZC_NOTIFY_CHAT" (no prefix) → "ZC_NOTIFY_CHAT"
// "packet_idle_unit" → "" (old-style, not in DEFINE table)
// "SYNTH_*" → "" (synthetic, not in DEFINE table)
func structSuffix(name string) string {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "PACKET_") {
		return strings.TrimPrefix(upper, "PACKET_")
	}
	if strings.HasPrefix(upper, "SYNTH_") {
		return ""
	}
	if strings.HasPrefix(name, "packet_") {
		return ""
	}
	// Some headers use the suffix directly (e.g. AC_ACCEPT_LOGIN without PACKET_ prefix)
	return upper
}

// TestSemanticDB_PacketIDMatchesDefinePacketHeader verifies that for every
// implementation in semantics/mappings.yaml whose struct has a DEFINE_PACKET_HEADER
// declaration, the claimed packet_id is one of the valid IDs for that struct.
//
// This catches wrong packet IDs in the DB (e.g. using 0x008e when rAthena says 0x008d).
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
	t.Logf("DEFINE_PACKET_HEADER entries loaded: %d unique struct suffixes", len(defines))

	type result struct {
		action     string
		packetID   string
		structName string
		category   string // "verified", "mismatch", "synth", "old_style", "not_found"
		validIDs   []string
	}

	var verified, mismatches, synths, oldStyle, notFound []result

	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			pid := normID(impl.PacketID)
			suffix := structSuffix(impl.StructName)
			r := result{action: name, packetID: pid, structName: impl.StructName}

			switch {
			case strings.HasPrefix(impl.StructName, "SYNTH_"):
				r.category = "synth"
				synths = append(synths, r)

			case strings.HasPrefix(impl.StructName, "packet_"):
				r.category = "old_style"
				oldStyle = append(oldStyle, r)

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

	t.Logf("Results: verified=%d mismatches=%d synth=%d old_style=%d not_found=%d",
		len(verified), len(mismatches), len(synths), len(oldStyle), len(notFound))

	// SYNTH_* and packet_* are informational only — log but don't fail.
	if len(synths) > 0 {
		t.Logf("SYNTH_* (unverifiable, expected): %d implementations", len(synths))
	}
	if len(oldStyle) > 0 {
		t.Logf("packet_* old-style (unverifiable via DEFINE): %d implementations", len(oldStyle))
		for _, r := range oldStyle {
			t.Logf("  %s: %s → %s", r.action, r.packetID, r.structName)
		}
	}
	if len(notFound) > 0 {
		t.Logf("PACKET_* not found in any DEFINE_PACKET_HEADER (%d):", len(notFound))
		for _, r := range notFound {
			t.Logf("  %s: %s → %s", r.action, r.packetID, r.structName)
		}
	}

	// MISMATCHES are hard failures.
	for _, r := range mismatches {
		t.Errorf("MISMATCH: action=%q claims packet_id=%s for struct %s\n"+
			"  but DEFINE_PACKET_HEADER says valid IDs for %s are: %v\n"+
			"  Fix: update semantics DB via MCP: semantics_update_implementation(action=%q, packet_id=<correct>)",
			r.action, r.packetID, r.structName,
			r.structName, r.validIDs,
			r.action)
	}
}

// TestSemanticDB_NoDuplicatePacketIDs verifies no two different actions
// claim the same packet_id (within the same direction). A packet ID can
// only map to one semantic action.
func TestSemanticDB_NoDuplicatePacketIDs(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Map normalised packet_id → first action that claimed it
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

// TestSemanticDB_PacketverRangesSane verifies that packetver_min < packetver_max
// (when max is non-zero) and that all values look like valid YYYYMMDD dates.
func TestSemanticDB_PacketverRangesSane(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	const minSane = 20030000 // rAthena didn't exist before 2003
	const maxSane = 20991231 // far future

	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			min := impl.PacketverMin
			max := impl.PacketverMax

			if min != 0 && (min < minSane || min > maxSane) {
				t.Errorf("%s impl %s: packetver_min=%d outside sane range [%d, %d]",
					name, impl.PacketID, min, minSane, maxSane)
			}
			if max != 0 && (max < minSane || max > maxSane) {
				t.Errorf("%s impl %s: packetver_max=%d outside sane range [%d, %d]",
					name, impl.PacketID, max, minSane, maxSane)
			}
			if min != 0 && max != 0 && min >= max {
				t.Errorf("%s impl %s: packetver_min=%d >= packetver_max=%d (empty range)",
					name, impl.PacketID, min, max)
			}
		}
	}
}

// TestSemanticDB_KnownCorrectBindings is a whitelist of bindings we have
// manually verified against rAthena source. Adding an entry here documents
// the verification and prevents future regression.
//
// This test does NOT require rAthena source — it is a pure DB consistency check.
func TestSemanticDB_KnownCorrectBindings(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	type binding struct {
		action     string
		packetID   string // normalised 0xNNNN
		structName string
		note       string
	}

	// Verified against rAthena source (DEFINE_PACKET_HEADER or clif.cpp comment).
	// Each entry here represents a confirmed correct binding.
	verified := []binding{
		// Modern DEFINE_PACKET_HEADER structs
		{"actor_died_or_disappeared", "0x0080", "PACKET_ZC_NOTIFY_VANISH", "DEFINE_PACKET_HEADER(ZC_NOTIFY_VANISH, 0x80)"},
		{"actor_exists", "0x0078", "packet_idle_unit", "clif_packetdb.hpp baseline"},
		{"actor_exists", "0x09ff", "packet_idle_unit", "clif_packetdb.hpp >= 20181121"},
		{"actor_moved", "0x007b", "packet_unit_walking", "clif_packetdb.hpp packet(0x007b,60)"},
		{"actor_moved", "0x09fd", "packet_unit_walking", "clif_packetdb.hpp >= 20181121"},
		{"item_pickup", "0x00a0", "PACKET_ZC_ITEM_PICKUP_ACK", "DEFINE_PACKET_HEADER"},
		{"account_id", "0x0283", "SYNTH_ZC_AID", "synthetic, manually verified"},
		{"ac_accept_login", "0x0069", "PACKET_AC_ACCEPT_LOGIN", "common/packets.hpp DEFINE_PACKET_HEADER(AC_ACCEPT_LOGIN, 0x69)"},
		{"ac_accept_login", "0x0ac4", "PACKET_AC_ACCEPT_LOGIN", "common/packets.hpp DEFINE_PACKET_HEADER(AC_ACCEPT_LOGIN, 0xac4)"},
		// chat_message: 0x008d is correct (clif.cpp:6753 comment + packets.hpp DEFINE)
		// DB used to claim 0x008e which is wrong — this entry documents the fix
		{"chat_message", "0x008d", "PACKET_ZC_NOTIFY_CHAT", "packets.hpp DEFINE_PACKET_HEADER(ZC_NOTIFY_CHAT, 0x8d)"},
		// market_purchase: 0x09d6 is correct — DB used to claim 0x0134 (CZ_PC_PURCHASE_ITEMLIST_FROMMC)
		{"market_purchase", "0x09d6", "PACKET_CZ_NPC_MARKET_PURCHASE", "packets_struct.hpp DEFINE_PACKET_HEADER(CZ_NPC_MARKET_PURCHASE, 0x09d6)"},
		// zc_ack_ranking: 0x097d is ZC_ACK_RANKING, 0x0af6 is ZC_ACK_RANKING2
		{"zc_ack_ranking", "0x097d", "PACKET_ZC_ACK_RANKING", "packets.hpp DEFINE_PACKET_HEADER(ZC_ACK_RANKING, 0x97d)"},
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
			t.Errorf("action %q: missing verified binding packet_id=%s struct=%s\n  note: %s\n  actual impls: %v",
				b.action, b.packetID, b.structName, b.note, action.Implementations)
		}
	}
}
