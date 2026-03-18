package preprocess

import (
	"regexp"
	"strconv"
	"strings"
)

// PacketEntry represents a single entry from clif_packetdb.hpp.
// Both packet() and parseable_packet() macros are parsed.
type PacketEntry struct {
	ID      uint16 // packet ID (e.g. 0x0085)
	Length  int16  // -1 for variable-length
	Handler string // handler function name (empty for packet() entries)
}

// rePacketdbAddpacket matches the preprocessed form:
//
//	packetdb_addpacket(0x0085, 5, clif_parse_WalkToXY, 2, 0)
//
// This is the form emitted after g++ -E -P expands the packet()/parseable_packet() macros.
var rePacketdbAddpacket = regexp.MustCompile(`packetdb_addpacket\s*\(\s*(0x[0-9A-Fa-f]+)\s*,\s*(-?\d+)(?:\s*,\s*(\w+))?`)

// rePacketEntry also matches the raw (pre-preprocessor) macro forms in case the
// raw file is passed instead of preprocessed output.
var rePacketEntry = regexp.MustCompile(`(?:packet|parseable_packet)\s*\(\s*(0x[0-9A-Fa-f]+)\s*,\s*(-?\d+)(?:\s*,\s*(\w+))?`)

// ParsePacketDB parses clif_packetdb.hpp content and returns all packet entries.
// The content should be the PREPROCESSED output of clif_packetdb.hpp (run through
// g++ -E -P at a specific PACKETVER) so that PACKETVER conditionals are resolved.
//
// After preprocessing, macros are expanded to packetdb_addpacket(...) calls.
// Duplicate entries (same ID, multiple handlers due to old shuffle tables) are all
// returned — callers should use the first entry for each handler as the canonical mapping.
func ParsePacketDB(content string) ([]PacketEntry, error) {
	var entries []PacketEntry
	for _, line := range strings.Split(content, "\n") {
		// Try preprocessed form first
		m := rePacketdbAddpacket.FindStringSubmatch(line)
		if m == nil {
			// Fall back to raw form
			m = rePacketEntry.FindStringSubmatch(line)
		}
		if m == nil {
			continue
		}
		idStr, lenStr, handler := m[1], m[2], m[3]
		id64, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(idStr), "0x"), 16, 16)
		if err != nil {
			continue
		}
		len64, err := strconv.ParseInt(lenStr, 10, 16)
		if err != nil {
			continue
		}
		entries = append(entries, PacketEntry{
			ID:      uint16(id64),
			Length:  int16(len64),
			Handler: handler,
		})
	}
	return entries, nil
}

// HandlerBaseIDs builds a map from handler name → first (base) packet ID assignment.
// It uses the FIRST occurrence of each handler in the packetdb entries, which corresponds
// to the base (unshuffled) packet ID assignment before any PACKETVER-conditional shuffles.
func HandlerBaseIDs(entries []PacketEntry) map[string]PacketEntry {
	m := make(map[string]PacketEntry)
	for _, e := range entries {
		if e.Handler == "" {
			continue
		}
		if _, already := m[e.Handler]; !already {
			m[e.Handler] = e
		}
	}
	return m
}

// ShuffleSection is one PACKETVER block from clif_shuffle.hpp.
// It maps handler names to their shuffled packet IDs for that version.
// Exact-match sections (PACKETVER == N) have RangeAbove == false.
// The final open-ended section (PACKETVER > N) has RangeAbove == true and
// PacketVer set to N (the lower bound, exclusive). Only one such section
// may exist in clif_shuffle.hpp and it is always last.
type ShuffleSection struct {
	PacketVer  uint32
	RangeAbove bool // true when the condition is PACKETVER > PacketVer
	// Entries maps handler name → shuffled packet entry for this version.
	// Multiple entries may share a handler (some handlers handle multiple sizes).
	Entries []PacketEntry
}

// reShuffleIf matches the #if PACKETVER == YYYYMMDD and #elif PACKETVER == YYYYMMDD lines.
var reShuffleIf = regexp.MustCompile(`#(?:if|elif)\s+PACKETVER\s*==\s*(\d{8})`)

// reShuffleGt matches the #elif PACKETVER > YYYYMMDD line (open-ended range).
var reShuffleGt = regexp.MustCompile(`#elif\s+PACKETVER\s*>\s*(\d{8})`)

// reIfAny matches any preprocessor conditional line that opens a new nesting level.
var reIfAny = regexp.MustCompile(`^\s*#if\b`)

// reEndif matches a bare #endif line.
var reEndif = regexp.MustCompile(`^\s*#endif\b`)

// ParseShuffle parses clif_shuffle.hpp (raw text) into a slice of ShuffleSections,
// one per PACKETVER block plus at most one PACKETVER > N range block.
// Sections are sorted by PacketVer ascending.
func ParseShuffle(content string) ([]ShuffleSection, error) {
	lines := strings.Split(content, "\n")
	var sections []ShuffleSection
	var cur *ShuffleSection
	depth := 0 // nesting depth of #if blocks inside the current section

	for _, line := range lines {
		// Range-above section (PACKETVER > N) — open-ended, must come after all == sections.
		if m := reShuffleGt.FindStringSubmatch(line); m != nil {
			if cur != nil {
				sections = append(sections, *cur)
			}
			pv64, _ := strconv.ParseUint(m[1], 10, 32)
			cur = &ShuffleSection{PacketVer: uint32(pv64), RangeAbove: true}
			depth = 0
			continue
		}
		// Exact-match section start (PACKETVER == N).
		if m := reShuffleIf.FindStringSubmatch(line); m != nil {
			if cur != nil {
				sections = append(sections, *cur)
			}
			pv64, _ := strconv.ParseUint(m[1], 10, 32)
			cur = &ShuffleSection{PacketVer: uint32(pv64)}
			depth = 0
			continue
		}
		// Track nested #if/#endif inside the current section.
		if cur != nil {
			if reIfAny.MatchString(line) {
				depth++
				continue
			}
			if reEndif.MatchString(line) {
				if depth > 0 {
					depth--
					continue
				}
				// depth == 0: this #endif closes the current top-level section.
				sections = append(sections, *cur)
				cur = nil
				continue
			}
		}
		if cur == nil {
			continue
		}
		// Parse parseable_packet lines within a section (including inside nested #if blocks).
		if m := rePacketEntry.FindStringSubmatch(line); m != nil {
			idStr, lenStr, handler := m[1], m[2], m[3]
			id64, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(idStr), "0x"), 16, 16)
			if err != nil {
				continue
			}
			len64, err := strconv.ParseInt(lenStr, 10, 16)
			if err != nil {
				continue
			}
			cur.Entries = append(cur.Entries, PacketEntry{
				ID:      uint16(id64),
				Length:  int16(len64),
				Handler: handler,
			})
		}
	}
	if cur != nil {
		sections = append(sections, *cur)
	}
	return sections, nil
}

// ObfuscationKey is one entry from clif_obfuscation.hpp.
type ObfuscationKey struct {
	PacketVer uint32
	Key0      uint32
	Key1      uint32
	Key2      uint32
}

// reObfKey matches the array initializer form produced by the packet_keys macro:
//
//	static uint32 clif_cryptKey[] = { 0xKEY0, 0xKEY1, 0xKEY2 };
var reObfKeys = regexp.MustCompile(`clif_cryptKey\[\]\s*=\s*\{\s*(0x[0-9A-Fa-f]+)\s*,\s*(0x[0-9A-Fa-f]+)\s*,\s*(0x[0-9A-Fa-f]+)`)

// ParseObfuscationKeys parses the preprocessed output of clif_obfuscation.hpp
// (already run with -DPACKET_OBFUSCATION and -DPACKETVER=N) and extracts the three keys.
// Returns (0,0,0) if no keys are found (obfuscation disabled for this PACKETVER).
func ParseObfuscationKeys(preprocessed string) (k0, k1, k2 uint32) {
	if m := reObfKeys.FindStringSubmatch(preprocessed); m != nil {
		parse := func(s string) uint32 {
			v, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 32)
			return uint32(v)
		}
		k0 = parse(m[1])
		k1 = parse(m[2])
		k2 = parse(m[3])
	}
	return
}
