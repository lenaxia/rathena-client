// Package main is the code generator entry point for rathena-client.
// It reads rAthena C++ headers via the GCC preprocessor and emits Go source
// files for pkg/session, pkg/events, pkg/decode, and pkg/encode.
//
// Usage:
//
//	go run ./internal/codegen/main.go \
//	    --rathena ~/personal/rathena \
//	    --out .
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lenaxia/rathena-client/internal/codegen/gen"
	"github.com/lenaxia/rathena-client/internal/codegen/preprocess"
	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

func main() {
	var (
		rathenaRoot   = flag.String("rathena", "", "path to rAthena repository root")
		outDir        = flag.String("out", ".", "output directory (repository root)")
		semanticsPath = flag.String("semantics", "semantics/mappings.yaml", "path to semantics/mappings.yaml")
	)
	flag.Parse()

	if *rathenaRoot == "" {
		log.Fatal("--rathena is required")
	}

	// Resolve stub paths relative to this binary's source location.
	// When run via `go run ./internal/codegen/main.go`, the working dir is the repo root.
	stubsDir := filepath.Join("internal", "codegen", "stubs")
	cfg := preprocess.Config{
		RathenaRoot:    *rathenaRoot,
		PacketsHPPStub: filepath.Join(stubsDir, "packets_hpp_stub.h"),
		CommonHPPStub:  filepath.Join(stubsDir, "common_hpp_stub.h"),
		SyntheticHPP:   filepath.Join(stubsDir, "synthetic_structs.hpp"),
	}

	if err := run(cfg, *outDir, *semanticsPath); err != nil {
		log.Fatal(err)
	}
}

func run(cfg preprocess.Config, outDir, semanticsPath string) error {
	log.Println("=== rathena-client codegen ===")

	// Step 1: Generate obfuscation keys (session/obfuscation_keys.go)
	log.Println("Generating obfuscation keys...")
	if err := genObfuscation(cfg, outDir); err != nil {
		return fmt.Errorf("obfuscation: %w", err)
	}
	log.Println("  → pkg/session/obfuscation_keys.go")

	// Step 2: Load semantic DB.
	log.Printf("Loading semantic DB from %s...", semanticsPath)
	db, err := semantics.LoadFile(semanticsPath)
	if err != nil {
		return fmt.Errorf("load semantics: %w", err)
	}
	log.Printf("  Loaded %d semantic actions", len(db.Actions))

	// Step 3: Build VersionTable from packets_struct.hpp.
	log.Println("Building VersionTable from packets_struct.hpp...")
	vt, err := buildVersionTable(cfg)
	if err != nil {
		return fmt.Errorf("build version table: %w", err)
	}
	log.Printf("  VersionTable has %d structs", len(vt))

	// Step 3b: Inject synthetic structs (SYNTH_*) for structless packets.
	log.Println("Injecting synthetic struct layouts...")
	if err := injectSynthetic(cfg, vt); err != nil {
		return fmt.Errorf("inject synthetic structs: %w", err)
	}
	log.Printf("  VersionTable now has %d structs (after synthetic injection)", len(vt))

	// Step 3c: Inject structs from common/packets.hpp (login/char server packets).
	// These are not in packets_struct.hpp so buildVersionTable never sees them.
	log.Println("Injecting common/packets.hpp struct layouts...")
	if err := injectCommonPacketStructs(cfg, vt); err != nil {
		return fmt.Errorf("inject common packet structs: %w", err)
	}
	log.Printf("  VersionTable now has %d structs (after common injection)", len(vt))

	// Step 3d: Inject structs from src/map/packets.hpp.
	// These are not in packets_struct.hpp so buildVersionTable never sees them.
	log.Println("Injecting map/packets.hpp struct layouts...")
	if err := injectMapPacketStructs(cfg, vt); err != nil {
		return fmt.Errorf("inject map packet structs: %w", err)
	}
	log.Printf("  VersionTable now has %d structs (after map injection)", len(vt))

	// Step 4: Generate length tables (session/lengths_*.go).
	// Must run after VersionTable is built so S→C lengths can be joined from struct sizes.
	log.Println("Generating length tables...")
	if err := genLengths(cfg, outDir, semanticsPath, vt, db); err != nil {
		return fmt.Errorf("lengths: %w", err)
	}
	log.Println("  → pkg/session/lengths_login.go, lengths_char.go, lengths_map.go")

	// Step 5: Generate event structs (pkg/events/*.go)
	log.Println("Generating event structs...")
	if err := genEvents(db, vt, outDir); err != nil {
		return fmt.Errorf("events: %w", err)
	}

	// Step 6: Generate send request types (pkg/send/*.go)
	log.Println("Generating send request types...")
	if err := genSend(db, vt, outDir); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	// Step 7: Generate decode functions (pkg/decode/*.go)
	log.Println("Generating decode functions...")
	if err := genDecode(db, vt, outDir); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	// Step 8: Generate encode functions (pkg/encode/*.go).
	// cleanGeneratedDir("pkg/encode") runs inside genEncode — it must complete
	// before genShuffle writes shuffle_map.go, or the clean sweep would delete it.
	//
	// We pre-build the shuffle base IDs so genEncode can validate that no generated
	// encoder hardcodes an ID that appears in the shuffle table (the root cause of
	// the encoder ID bugs documented in worklogs 0069–0073).
	log.Println("Building shuffle base IDs for encoder validation...")
	shuffleBaseIDs, err := buildShuffleBaseIDs(cfg)
	if err != nil {
		// Non-fatal: if clif_shuffle.hpp is unavailable, skip the validation.
		log.Printf("  WARNING: could not build shuffle base IDs (%v) — skipping shuffle overlap check", err)
		shuffleBaseIDs = nil
	} else {
		log.Printf("  %d shuffle base IDs loaded for validation", len(shuffleBaseIDs))
	}

	log.Println("Generating encode functions...")
	if err := genEncodeWithShuffleCheck(db, vt, outDir, shuffleBaseIDs); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	// Step 9: Generate shuffle table (encode/shuffle_map.go).
	// Must run AFTER genEncode because genEncode calls cleanGeneratedDir("pkg/encode")
	// which deletes all "// Code generated" files. Writing shuffle_map.go after the
	// clean sweep ensures it is never deleted.
	log.Println("Generating shuffle table...")
	if err := genShuffle(cfg, outDir); err != nil {
		return fmt.Errorf("shuffle: %w", err)
	}
	log.Println("  → pkg/encode/shuffle_map.go")

	// Step 10: Generate SemanticAction enum (pkg/session/actions.go)
	log.Println("Generating SemanticAction enum...")
	if err := genActions(db, outDir); err != nil {
		return fmt.Errorf("actions: %w", err)
	}
	log.Println("  → pkg/session/actions.go")

	// Step 11: Generate receive dispatch table (pkg/session/receive_dispatch.go)
	log.Println("Generating receive dispatch table...")
	if err := genReceiveDispatch(db, vt, outDir); err != nil {
		return fmt.Errorf("receive_dispatch: %w", err)
	}
	log.Println("  → pkg/session/receive_dispatch.go")

	// Step 12: Generate send encoder registration (pkg/encode/register.go)
	log.Println("Generating send encoder registration...")
	if err := genRegister(db, vt, outDir); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	log.Println("  → pkg/encode/register.go")

	log.Println("=== Done ===")
	return nil
}

// buildVersionTable runs GCC at all PACKETVER breakpoints from packets_struct.hpp
// and builds the complete VersionTable.
func buildVersionTable(cfg preprocess.Config) (preprocess.VersionTable, error) {
	// Extract all breakpoints from packets_struct.hpp.
	structFile := filepath.Join(cfg.RathenaRoot, "src", "map", "packets_struct.hpp")
	dates, err := preprocess.ExtractBreakpointsFromFile(structFile)
	if err != nil {
		return nil, fmt.Errorf("extract breakpoints: %w", err)
	}
	// Add a baseline.
	dates = preprocess.SortBreakpoints(append([]uint32{20030000}, dates...))

	log.Printf("  %d breakpoints to process", len(dates))

	var versionedDBs []preprocess.VersionedDB
	for _, pv := range dates {
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourcePacketsStruct, pv)
		if err != nil {
			log.Printf("  WARNING: preprocess at %d failed: %v", pv, err)
			continue
		}
		db, err := preprocess.ExtractStructs(preprocessed, pv)
		if err != nil {
			log.Printf("  WARNING: ExtractStructs at %d failed: %v", pv, err)
			continue
		}
		versionedDBs = append(versionedDBs, preprocess.VersionedDB{Ver: pv, DB: db})
	}

	if len(versionedDBs) == 0 {
		return nil, fmt.Errorf("no struct data extracted")
	}

	return preprocess.BuildVersionTable(versionedDBs)
}

// injectSynthetic adds SYNTH_* struct layouts into an existing VersionTable.
// Called after buildVersionTable so synthetic structs are available for codegen.
func injectSynthetic(cfg preprocess.Config, vt preprocess.VersionTable) error {
	return preprocess.InjectSyntheticStructs(cfg, vt)
}

// injectCommonPacketStructs processes common/packets.hpp at all its PACKETVER
// breakpoints and injects all structs matching the target prefixes (PACKET_AC_,
// PACKET_HC_, PACKET_SC_, PACKET_TC_, PACKET_CT_) into the VersionTable. This is
// necessary because common/packets.hpp is not processed by buildVersionTable (which
// only reads packets_struct.hpp from the map server).
func injectCommonPacketStructs(cfg preprocess.Config, vt preprocess.VersionTable) error {
	commonFile := filepath.Join(cfg.RathenaRoot, "src", "common", "packets.hpp")
	allDates, err := preprocess.ExtractBreakpointsFromFile(commonFile)
	if err != nil {
		return fmt.Errorf("extract common breakpoints: %w", err)
	}
	allDates = preprocess.SortBreakpoints(append([]uint32{20030000}, allDates...))

	type versionedDB struct {
		ver uint32
		db  preprocess.StructDB
	}
	var snapshots []versionedDB
	for _, pv := range allDates {
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourceCommonPackets, pv)
		if err != nil {
			log.Printf("  WARNING: preprocess common/packets.hpp at %d failed: %v", pv, err)
			continue
		}
		db, err := preprocess.ExtractStructs(preprocessed, pv)
		if err != nil {
			log.Printf("  WARNING: ExtractStructs common at %d failed: %v", pv, err)
			continue
		}
		snapshots = append(snapshots, versionedDB{ver: pv, db: db})
	}

	if len(snapshots) == 0 {
		return fmt.Errorf("injectCommonPacketStructs: no snapshots extracted from common/packets.hpp")
	}

	// Collect all struct names matching target prefixes across all snapshots.
	// PACKET_CA_ = Client→Auth (login server C→S, e.g. CA_LOGIN, CA_LOGIN2)
	// PACKET_AC_ = Auth→Client (login server S→C)
	// PACKET_CH_ = Client→Char (char server C→S, e.g. CH_SELECT_CHAR, CH_CHARLIST_REQ)
	// PACKET_HC_ = Char→Client (char server S→C)
	// PACKET_SC_ = shared disconnect notification (login + char + map servers)
	// PACKET_TC_ / PACKET_CT_ = OTP/token auth packets
	commonPrefixes := []string{"PACKET_CA_", "PACKET_AC_", "PACKET_CH_", "PACKET_HC_", "PACKET_SC_", "PACKET_TC_", "PACKET_CT_"}
	structNames := make(map[string]bool)
	for _, snap := range snapshots {
		for name := range snap.db {
			for _, prefix := range commonPrefixes {
				if strings.HasPrefix(name, prefix) {
					structNames[name] = true
					break
				}
			}
		}
	}

	injected := 0
	for structName := range structNames {
		// Collect version ranges where this struct appears and has a distinct layout.
		var ranges []preprocess.VersionedLayout
		for i, snap := range snapshots {
			layout, ok := snap.db[structName]
			if !ok || layout == nil || !layout.Available {
				continue
			}
			// Determine the MaxVer for this range: the MinVer of the next snapshot that
			// has a different layout (or 0 if this is the last one).
			minVer := snap.ver
			maxVer := uint32(0)
			for j := i + 1; j < len(snapshots); j++ {
				next, ok := snapshots[j].db[structName]
				if !ok || next == nil || !next.Available {
					continue
				}
				if next.TotalSize != layout.TotalSize || len(next.Fields) != len(layout.Fields) {
					maxVer = snapshots[j].ver
					break
				}
			}
			// Only add a new range if it has a different layout than the previous.
			if len(ranges) > 0 {
				prev := ranges[len(ranges)-1]
				if prev.Layout != nil && prev.Layout.TotalSize == layout.TotalSize &&
					len(prev.Layout.Fields) == len(layout.Fields) {
					// Same layout — extend previous range's MaxVer.
					ranges[len(ranges)-1].MaxVer = maxVer
					continue
				}
			}
			l := *layout
			ranges = append(ranges, preprocess.VersionedLayout{
				MinVer: minVer,
				MaxVer: maxVer,
				Layout: &l,
			})
		}
		if len(ranges) == 0 {
			log.Printf("  WARNING: struct %s not found in any common/packets.hpp snapshot", structName)
			continue
		}
		vt[structName] = ranges
		injected++
		log.Printf("  Injected %s: %d version range(s)", structName, len(ranges))
	}

	return nil
}

// injectMapPacketStructs processes src/map/packets.hpp at all its PACKETVER
// breakpoints and injects all structs matching the target prefixes (PACKET_ZC_,
// PACKET_SC_) into the VersionTable. This is necessary because packets.hpp is not
// processed by buildVersionTable (which only reads packets_struct.hpp from the map server).
func injectMapPacketStructs(cfg preprocess.Config, vt preprocess.VersionTable) error {
	packetsFile := filepath.Join(cfg.RathenaRoot, "src", "map", "packets.hpp")
	allDates, err := preprocess.ExtractBreakpointsFromFile(packetsFile)
	if err != nil {
		return fmt.Errorf("extract map/packets.hpp breakpoints: %w", err)
	}
	allDates = preprocess.SortBreakpoints(append([]uint32{20030000}, allDates...))

	type versionedDB struct {
		ver uint32
		db  preprocess.StructDB
	}
	var snapshots []versionedDB
	for _, pv := range allDates {
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourcePackets, pv)
		if err != nil {
			log.Printf("  WARNING: preprocess map/packets.hpp at %d failed: %v", pv, err)
			continue
		}
		db, err := preprocess.ExtractStructs(preprocessed, pv)
		if err != nil {
			log.Printf("  WARNING: ExtractStructs map/packets.hpp at %d failed: %v", pv, err)
			continue
		}
		snapshots = append(snapshots, versionedDB{ver: pv, db: db})
	}

	if len(snapshots) == 0 {
		return fmt.Errorf("injectMapPacketStructs: no snapshots extracted from map/packets.hpp")
	}

	// Collect all struct names matching target prefixes across all snapshots.
	mapPrefixes := []string{"PACKET_ZC_", "PACKET_SC_", "PACKET_CZ_"}
	structNames := make(map[string]bool)
	for _, snap := range snapshots {
		for name := range snap.db {
			for _, prefix := range mapPrefixes {
				if strings.HasPrefix(name, prefix) {
					structNames[name] = true
					break
				}
			}
		}
	}

	injected := 0
	skippedExisting := 0
	for structName := range structNames {
		// If this struct already has a VT entry from packets_struct.hpp, keep
		// that authoritative version. packets_struct.hpp's PACKETVER guards are
		// more fine-grained than map/packets.hpp's, so overwriting would
		// coarsen the layout ranges — causing correct-at-source layouts to be
		// misattributed to later packets.hpp breakpoints (e.g. ZC_USESKILL_ACK
		// changes at pv 20181212 in packets_struct.hpp but the packets.hpp
		// breakpoint that first observes the new layout is 20191120, so
		// injecting from packets.hpp would misdate the transition).
		//
		// Only inject structs that ORIGINATE from map/packets.hpp — i.e. those
		// not already present in the VT.
		if existing, ok := vt[structName]; ok && len(existing) > 0 {
			skippedExisting++
			continue
		}
		// Collect version ranges where this struct appears and has a distinct layout.
		var ranges []preprocess.VersionedLayout
		for i, snap := range snapshots {
			layout, ok := snap.db[structName]
			if !ok || layout == nil || !layout.Available {
				continue
			}
			// Determine the MaxVer for this range: the MinVer of the next snapshot that
			// has a different layout (or 0 if this is the last one).
			minVer := snap.ver
			maxVer := uint32(0)
			for j := i + 1; j < len(snapshots); j++ {
				next, ok := snapshots[j].db[structName]
				if !ok || next == nil || !next.Available {
					continue
				}
				if next.TotalSize != layout.TotalSize || len(next.Fields) != len(layout.Fields) {
					maxVer = snapshots[j].ver
					break
				}
			}
			// Only add a new range if it has a different layout than the previous.
			if len(ranges) > 0 {
				prev := ranges[len(ranges)-1]
				if prev.Layout != nil && prev.Layout.TotalSize == layout.TotalSize &&
					len(prev.Layout.Fields) == len(layout.Fields) {
					// Same layout — extend previous range's MaxVer.
					ranges[len(ranges)-1].MaxVer = maxVer
					continue
				}
			}
			l := *layout
			ranges = append(ranges, preprocess.VersionedLayout{
				MinVer: minVer,
				MaxVer: maxVer,
				Layout: &l,
			})
		}
		if len(ranges) == 0 {
			log.Printf("  WARNING: struct %s not found in any map/packets.hpp snapshot", structName)
			continue
		}
		vt[structName] = ranges
		injected++
		log.Printf("  Injected %s: %d version range(s)", structName, len(ranges))
	}
	if skippedExisting > 0 {
		log.Printf("  Skipped %d structs already present in VT (packets_struct.hpp is authoritative)", skippedExisting)
	}

	return nil
}

func genShuffle(cfg preprocess.Config, outDir string) error {
	// Read clif_shuffle.hpp (raw — no preprocessing needed for shuffle sections).
	shuffleFile := filepath.Join(cfg.RathenaRoot, "src", "map", "clif_shuffle.hpp")
	content, err := os.ReadFile(shuffleFile)
	if err != nil {
		return fmt.Errorf("read clif_shuffle.hpp: %w", err)
	}
	sections, err := preprocess.ParseShuffle(string(content))
	if err != nil {
		return fmt.Errorf("parse shuffle: %w", err)
	}
	log.Printf("  Parsed %d shuffle sections", len(sections))

	// Preprocess clif_packetdb.hpp at a modern PACKETVER to get the base IDs.
	// We use the first PACKETVER where 0x0436 (CZ_ENTER2) exists, which is
	// after the old packet IDs era. Any modern PACKETVER works for base ID lookup.
	packetdbPreprocessed, err := preprocess.Preprocess(cfg, preprocess.SourceClifPacketDB, 20180307)
	if err != nil {
		return fmt.Errorf("preprocess packetdb: %w", err)
	}
	entries, err := preprocess.ParsePacketDB(packetdbPreprocessed)
	if err != nil {
		return fmt.Errorf("parse packetdb: %w", err)
	}
	baseIDs := preprocess.HandlerBaseIDs(entries)
	log.Printf("  %d handler base IDs", len(baseIDs))

	breakpoints := gen.BuildShuffleBreakpoints(sections, baseIDs)
	log.Printf("  %d non-trivial shuffle breakpoints", len(breakpoints))

	src, err := gen.GenerateShuffleFile(breakpoints)
	if err != nil {
		return err
	}

	return writeFile(filepath.Join(outDir, "pkg", "encode", "shuffle_map.go"), src)
}

func genObfuscation(cfg preprocess.Config, outDir string) error {
	// Extract obfuscation keys from all PACKETVER dates found in clif_obfuscation.hpp.
	obfFile := filepath.Join(cfg.RathenaRoot, "src", "map", "clif_obfuscation.hpp")
	dates, err := preprocess.ExtractBreakpointsFromFile(obfFile)
	if err != nil {
		return fmt.Errorf("extract obf breakpoints: %w", err)
	}
	log.Printf("  %d obfuscation breakpoints", len(dates))

	var keys []gen.ObfuscationKeySet
	for _, pv := range dates {
		out, err := preprocess.Preprocess(cfg, preprocess.SourceClifObfuscation, pv)
		if err != nil {
			// clif_obfuscation.hpp has #error guards for unsupported PACKETVERs — skip them.
			log.Printf("  WARNING: skipping obfuscation at %d: %v", pv, err)
			continue
		}
		k0, k1, k2 := preprocess.ParseObfuscationKeys(out)
		if k0|k1|k2 == 0 {
			continue // no keys for this packetver
		}
		keys = append(keys, gen.ObfuscationKeySet{
			PacketVer: pv,
			Key0:      k0,
			Key1:      k1,
			Key2:      k2,
		})
	}
	log.Printf("  %d PACKETVERs with non-zero obfuscation keys", len(keys))

	src, err := gen.GenerateObfuscationFile(keys)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "pkg", "session", "obfuscation_keys.go"), src)
}

func genLengths(cfg preprocess.Config, outDir, semanticsPath string, vt preprocess.VersionTable, db *semantics.DB) error {
	// --- Map server lengths ---
	// Part 1: C→S lengths from clif_packetdb.hpp (existing).
	mapBreakpoints, err := buildMapLengthBreakpoints(cfg)
	if err != nil {
		return fmt.Errorf("build map length breakpoints: %w", err)
	}

	// Part 2: S→C lengths from packets.hpp HEADER_* constants (Gap B).
	// Use fill-only merge: clif_packetdb lengths (Part 1) take priority.
	// In particular, packets registered as variable-length (-1) in clif_packetdb
	// must not be overridden with a fixed struct size from packets.hpp.
	stocPacketsHPP, err := buildMapStocLengthBreakpoints(cfg)
	if err != nil {
		return fmt.Errorf("build map s→c packets.hpp breakpoints: %w", err)
	}
	mapBreakpoints = mergeBreakpointsFillOnly(mapBreakpoints, stocPacketsHPP)

	// Part 3: S→C lengths joined from VersionTable + SemanticDB mappings: section (Gap A + Gap C).
	packetMappings, err := semantics.LoadMappings(semanticsPath)
	if err != nil {
		return fmt.Errorf("load packet mappings for join pass: %w", err)
	}
	log.Printf("  Loaded %d packet mappings for S→C join pass", len(packetMappings))
	stocJoin, err := buildMapStocJoinPass(vt, packetMappings)
	if err != nil {
		return fmt.Errorf("build map s→c join pass: %w", err)
	}
	mapBreakpoints = mergeBreakpointsFillOnly(mapBreakpoints, stocJoin)

	// Part 4: enum-assigned packet IDs from packets_struct.hpp (Gap D).
	// Some packet IDs are assigned via C++ enum values (e.g. inventorylistnormalType,
	// sendLookType) rather than DEFINE_PACKET_HEADER constants. These are invisible to
	// Parts 1–3. We parse packets_struct.hpp explicitly to extract them.
	enumBPs, err := buildMapEnumPacketBreakpoints(cfg)
	if err != nil {
		return fmt.Errorf("build map enum packet breakpoints: %w", err)
	}
	mapBreakpoints = mergeBreakpoints(mapBreakpoints, enumBPs)
	mapBreakpoints = deduplicateLengthBreakpoints(mapBreakpoints)
	_ = db

	mapSrc, err := gen.GenerateLengthsFile(gen.ServerMap, mapBreakpoints)
	if err != nil {
		return fmt.Errorf("generate map lengths: %w", err)
	}
	if err := writeFile(filepath.Join(outDir, "pkg", "session", "lengths_map.go"), mapSrc); err != nil {
		return err
	}

	// --- Login and char server lengths (from common/packets.hpp) ---
	loginBreakpoints, charBreakpoints, err := buildLoginCharLengthBreakpoints(cfg)
	if err != nil {
		return fmt.Errorf("build login/char length breakpoints: %w", err)
	}

	loginSrc, err := gen.GenerateLengthsFile(gen.ServerLogin, loginBreakpoints)
	if err != nil {
		return fmt.Errorf("generate login lengths: %w", err)
	}
	if err := writeFile(filepath.Join(outDir, "pkg", "session", "lengths_login.go"), loginSrc); err != nil {
		return err
	}

	charSrc, err := gen.GenerateLengthsFile(gen.ServerChar, charBreakpoints)
	if err != nil {
		return fmt.Errorf("generate char lengths: %w", err)
	}
	return writeFile(filepath.Join(outDir, "pkg", "session", "lengths_char.go"), charSrc)
}

// buildMapLengthBreakpoints extracts packet lengths from clif_packetdb.hpp for the
// map server, returning a sorted slice of LengthBreakpoint diffs.
func buildMapLengthBreakpoints(cfg preprocess.Config) ([]gen.LengthBreakpoint, error) {
	packetdbFile := filepath.Join(cfg.RathenaRoot, "src", "map", "clif_packetdb.hpp")
	allDates, err := preprocess.ExtractBreakpointsFromFile(packetdbFile)
	if err != nil {
		return nil, fmt.Errorf("extract packetdb breakpoints: %w", err)
	}
	allDates = append([]uint32{1}, allDates...)
	allDates = preprocess.SortBreakpoints(allDates)
	log.Printf("  %d packetdb breakpoints to process", len(allDates))

	type lenTable map[uint16]int16
	var prev lenTable
	var mapBreakpoints []gen.LengthBreakpoint

	for i, pv := range allDates {
		if pv == 1 {
			pv = 20030000
		}
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourceClifPacketDB, pv)
		if err != nil {
			return nil, fmt.Errorf("preprocess packetdb at %d: %w", allDates[i], err)
		}
		entries, err := preprocess.ParsePacketDB(preprocessed)
		if err != nil {
			return nil, fmt.Errorf("parse packetdb at %d: %w", allDates[i], err)
		}

		cur := make(lenTable)
		for _, e := range entries {
			cur[e.ID] = e.Length
		}

		var changed []gen.LengthEntry
		for id, length := range cur {
			if prev == nil || prev[id] != length {
				changed = append(changed, gen.LengthEntry{ID: id, Length: length})
			}
		}
		for id := range prev {
			if _, ok := cur[id]; !ok {
				changed = append(changed, gen.LengthEntry{ID: id, Length: 0})
			}
		}

		if len(changed) > 0 {
			ver := allDates[i]
			if ver == 1 {
				ver = 0
			}
			mapBreakpoints = append(mapBreakpoints, gen.LengthBreakpoint{Ver: ver, Entries: changed})
		}
		prev = cur
	}
	return mapBreakpoints, nil
}

// loginPrefixes is the set of packet name prefixes that belong to the login server.
// CA_ = Client→Auth, AC_ = Auth→Client.
// CT_ / TC_ = OTP/token auth packets, used exclusively in loginclif.cpp.
// SC_ = shared disconnect notification, used on login, char AND map servers.
var loginPrefixes = map[string]bool{
	"CA": true,
	"AC": true,
	"CT": true,
	"TC": true,
	"SC": true, // SC_NOTIFY_BAN is sent by loginclif.cpp, charserv_clif.cpp, and clif.cpp
}

// charPrefixes is the set of packet name prefixes that belong to the char server.
// CH_ = Client→Char, HC_ = Char→Client.
// SC_ = shared disconnect notification, also used on char server (charserv_clif.cpp:514).
// PING = keep-alive shared between login and char sessions.
var charPrefixes = map[string]bool{
	"CH":   true,
	"HC":   true,
	"SC":   true, // SC_NOTIFY_BAN also used by char server
	"PING": true,
}

// buildLoginCharLengthBreakpoints extracts packet lengths from common/packets.hpp
// and splits them into login-server and char-server LengthBreakpoint slices.
// A packet may appear in both tables if its prefix is in both loginPrefixes and charPrefixes.
func buildLoginCharLengthBreakpoints(cfg preprocess.Config) (loginBPs, charBPs []gen.LengthBreakpoint, err error) {
	commonFile := filepath.Join(cfg.RathenaRoot, "src", "common", "packets.hpp")
	allDates, err := preprocess.ExtractBreakpointsFromFile(commonFile)
	if err != nil {
		return nil, nil, fmt.Errorf("extract common breakpoints: %w", err)
	}
	// Always include a baseline at the very start.
	allDates = append([]uint32{20030000}, allDates...)
	allDates = preprocess.SortBreakpoints(allDates)
	log.Printf("  %d common/packets.hpp breakpoints to process", len(allDates))

	type lenTable map[uint16]int16
	var prevLogin, prevChar lenTable

	for _, pv := range allDates {
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourceCommonPackets, pv)
		if err != nil {
			log.Printf("  WARNING: preprocess common/packets.hpp at %d failed: %v", pv, err)
			continue
		}
		structDB, err := preprocess.ExtractStructs(preprocessed, pv)
		if err != nil {
			log.Printf("  WARNING: ExtractStructs common at %d failed: %v", pv, err)
			continue
		}
		packets := preprocess.ParseCommonPacketHeaders(preprocessed, structDB)

		curLogin := make(lenTable)
		curChar := make(lenTable)
		for _, p := range packets {
			// A packet may belong to both tables (e.g. SC_NOTIFY_BAN).
			if loginPrefixes[p.Prefix] {
				curLogin[p.ID] = p.Length
			}
			if charPrefixes[p.Prefix] {
				curChar[p.ID] = p.Length
			}
		}

		// The first breakpoint (baseline) is always emitted as Ver=0 (unconditional).
		// Subsequent breakpoints are emitted as `if pv >= N`.
		ver := pv
		if pv == 20030000 {
			ver = 0
		}

		loginChanged := diffLenTable(prevLogin, curLogin)
		if len(loginChanged) > 0 {
			loginBPs = append(loginBPs, gen.LengthBreakpoint{Ver: ver, Entries: loginChanged})
		}
		charChanged := diffLenTable(prevChar, curChar)
		if len(charChanged) > 0 {
			charBPs = append(charBPs, gen.LengthBreakpoint{Ver: ver, Entries: charChanged})
		}

		prevLogin = curLogin
		prevChar = curChar
	}
	return loginBPs, charBPs, nil
}

// diffLenTable returns the entries that changed between prev and cur.
// Entries present in cur but missing/different in prev are returned.
// Entries removed from cur (present in prev, absent in cur) are returned with Length=0.
func diffLenTable(prev, cur map[uint16]int16) []gen.LengthEntry {
	var changed []gen.LengthEntry
	for id, length := range cur {
		if prev == nil || prev[id] != length {
			changed = append(changed, gen.LengthEntry{ID: id, Length: length})
		}
	}
	for id := range prev {
		if _, ok := cur[id]; !ok {
			changed = append(changed, gen.LengthEntry{ID: id, Length: 0})
		}
	}
	return changed
}

// buildMapStocLengthBreakpoints processes src/map/packets.hpp at each PACKETVER
// breakpoint and extracts S→C packet lengths via ParseCommonPacketHeaders.
// This covers Gap B: packets that have HEADER_* constants and struct definitions
// in packets.hpp but are not registered in clif_packetdb.hpp (or are S→C only).
func buildMapStocLengthBreakpoints(cfg preprocess.Config) ([]gen.LengthBreakpoint, error) {
	packetsFile := filepath.Join(cfg.RathenaRoot, "src", "map", "packets.hpp")
	allDates, err := preprocess.ExtractBreakpointsFromFile(packetsFile)
	if err != nil {
		return nil, fmt.Errorf("extract packets.hpp breakpoints: %w", err)
	}
	allDates = preprocess.SortBreakpoints(append([]uint32{20030000}, allDates...))
	log.Printf("  %d packets.hpp breakpoints to process", len(allDates))

	type lenTable map[uint16]int16
	var prev lenTable
	var bps []gen.LengthBreakpoint

	for _, pv := range allDates {
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourcePackets, pv)
		if err != nil {
			log.Printf("  WARNING: preprocess packets.hpp at %d failed: %v", pv, err)
			continue
		}
		structDB, err := preprocess.ExtractStructs(preprocessed, pv)
		if err != nil {
			log.Printf("  WARNING: ExtractStructs packets.hpp at %d failed: %v", pv, err)
			continue
		}
		packets := preprocess.ParseCommonPacketHeaders(preprocessed, structDB)

		cur := make(lenTable)
		for _, p := range packets {
			cur[p.ID] = p.Length
		}

		ver := pv
		if pv == 20030000 {
			ver = 0
		}

		changed := diffLenTable(prev, cur)
		if len(changed) > 0 {
			bps = append(bps, gen.LengthBreakpoint{Ver: ver, Entries: changed})
		}
		prev = cur
	}
	return bps, nil
}

// reEnumAssign matches a single-line enum value assignment in preprocessed C++:
//
//	inventorylistnormalType = 0xb09,
//	sendLookType = 0x1d7,
var reEnumAssign = regexp.MustCompile(`^\s*(\w+)\s*=\s*(0x[0-9A-Fa-f]+)\s*,`)

// enumPacketEntry describes a known enum-assigned packet ID in packets_struct.hpp
// and the struct whose sizeof() gives the wire length (empty structName = variable-length).
type enumPacketEntry struct {
	enumName         string   // C++ enum value name, e.g. "inventorylistnormalType"
	structName       string   // PACKET_* struct name for size, "" = variable-length
	structCandidates []string // alternative struct names tried in order when struct varies by PACKETVER
	varLen           bool     // if true, packet is variable-length (length field in bytes [2:4])
}

// knownEnumPackets is the table of enum-assigned packet IDs that the HEADER_*
// parser in ParseCommonPacketHeaders cannot see. Each entry maps an enum name
// to its struct (for fixed-size) or marks it as variable-length.
//
// Verified against packets_struct.hpp:
//   - inventorylistnormalType / inventorylistequipType: packet_itemlist_normal/equip,
//     which carry a packetLength field → variable-length.
//   - sendLookType: PACKET_ZC_SPRITE_CHANGE, fixed size but changes at PACKETVER >= 20181121
//     (uint32 val/val2 replaces uint16). clif_packetdb.hpp is not updated so stays 11;
//     structDB gives the correct size.
//   - cartlistnormalType / cartlistequipType: packet_itemlist_normal/equip, variable-length.
//   - storageListNormalType / storageListEquipType: same structs as cartlist, variable-length.
//   - skillscale: PACKET_ZC_SKILL_SCALE, fixed size.
//   - partymemberinfo: PACKET_ZC_ADD_MEMBER_TO_GROUP, fixed size (versioned fields).
//   - partyinfo: PACKET_ZC_GROUP_LIST, variable-length (has packetLen).
//   - guildLeave: PACKET_ZC_ACK_LEAVE_GUILD1/2 (versioned, tries new→old).
//   - guildExpulsion: PACKET_ZC_ACK_BAN_GUILD1/2/3 (versioned, tries new→old).
//   - graffiti_entryType: packet_graffiti_entry, fixed size.
//   - roulettgenerateackType: packet_roulette_generate_ack, fixed size (versioned itemId).
//   - useItemAckType: PACKET_ZC_USE_ITEM_ACK, fixed size (versioned fields).
//   - authError: PACKET_ZC_REFUSE_LOGIN, fixed size. Note: struct may use different name per version;
//     clif_packetdb already tracks 0x006a and 0x083e; Part 4 covers 0x0b02.
var knownEnumPackets = []enumPacketEntry{
	// Original three entries
	{enumName: "inventorylistnormalType", varLen: true},
	{enumName: "inventorylistequipType", varLen: true},
	{enumName: "sendLookType", structName: "PACKET_ZC_SPRITE_CHANGE"},
	// Cart and storage list packets (same struct type as inventory, variable-length)
	{enumName: "cartlistnormalType", varLen: true},
	{enumName: "cartlistequipType", varLen: true},
	{enumName: "storageListNormalType", varLen: true},
	{enumName: "storageListEquipType", varLen: true},
	// Fixed-size packets with versioned struct sizes
	{enumName: "skillscale", structName: "PACKET_ZC_SKILL_SCALE"},
	{enumName: "partymemberinfo", structName: "PACKET_ZC_ADD_MEMBER_TO_GROUP"},
	{enumName: "graffiti_entryType", structName: "packet_graffiti_entry"},
	{enumName: "roulettgenerateackType", structName: "packet_roulette_generate_ack"},
	{enumName: "useItemAckType", structName: "PACKET_ZC_USE_ITEM_ACK"},
	// Variable-length party list
	{enumName: "partyinfo", varLen: true},
	// Guild leave/expulsion: struct name varies by PACKETVER — try newest→oldest
	{enumName: "guildLeave", structCandidates: []string{
		"PACKET_ZC_ACK_LEAVE_GUILD2", // PACKETVER >= 20161019
		"PACKET_ZC_ACK_LEAVE_GUILD1", // PACKETVER < 20161019
	}},
	{enumName: "guildExpulsion", structCandidates: []string{
		"PACKET_ZC_ACK_BAN_GUILD3", // PACKETVER >= 20161019
		"PACKET_ZC_ACK_BAN_GUILD2", // PACKETVER >= 20100803
		"PACKET_ZC_ACK_BAN_GUILD1", // PACKETVER < 20100803
	}},
}

// buildMapEnumPacketBreakpoints processes packets_struct.hpp at each of its
// PACKETVER breakpoints and emits LengthBreakpoints for enum-assigned packet IDs
// that are invisible to ParseCommonPacketHeaders (Gap D).
func buildMapEnumPacketBreakpoints(cfg preprocess.Config) ([]gen.LengthBreakpoint, error) {
	structFile := filepath.Join(cfg.RathenaRoot, "src", "map", "packets_struct.hpp")
	allDates, err := preprocess.ExtractBreakpointsFromFile(structFile)
	if err != nil {
		return nil, fmt.Errorf("extract packets_struct.hpp breakpoints: %w", err)
	}
	allDates = preprocess.SortBreakpoints(append([]uint32{20030000}, allDates...))
	log.Printf("  %d packets_struct.hpp breakpoints to process for enum packets", len(allDates))

	type lenTable map[uint16]int16
	var prev lenTable
	var bps []gen.LengthBreakpoint

	for _, pv := range allDates {
		preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourcePacketsStruct, pv)
		if err != nil {
			log.Printf("  WARNING: preprocess packets_struct.hpp at %d failed: %v", pv, err)
			continue
		}
		structDB, err := preprocess.ExtractStructs(preprocessed, pv)
		if err != nil {
			log.Printf("  WARNING: ExtractStructs packets_struct.hpp at %d failed: %v", pv, err)
			continue
		}

		// Extract enum assignments for the known enum names.
		enumVals := make(map[string]uint16)
		for _, line := range strings.Split(preprocessed, "\n") {
			m := reEnumAssign.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name := m[1]
			for _, e := range knownEnumPackets {
				if e.enumName == name {
					id64, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(m[2]), "0x"), 16, 16)
					if err == nil {
						enumVals[name] = uint16(id64)
					}
					break
				}
			}
		}

		// Build the current length table from enum assignments.
		cur := make(lenTable)
		for _, e := range knownEnumPackets {
			id, ok := enumVals[e.enumName]
			if !ok {
				continue
			}
			var length int16
			if e.varLen {
				length = -1
			} else if len(e.structCandidates) > 0 {
				// Try each candidate struct name in order; use the first one found.
				found := false
				for _, sn := range e.structCandidates {
					if layout, ok2 := structDB[sn]; ok2 && layout != nil && layout.Available {
						length = int16(layout.TotalSize)
						found = true
						break
					}
				}
				if !found {
					continue // none of the candidates available at this PACKETVER
				}
			} else if e.structName != "" {
				if layout, ok2 := structDB[e.structName]; ok2 && layout != nil && layout.Available {
					length = int16(layout.TotalSize)
				} else {
					continue // unknown size — skip
				}
			} else {
				continue
			}
			cur[id] = length
		}

		ver := pv
		if pv == 20030000 {
			ver = 0
		}

		changed := diffLenTable(prev, cur)
		if len(changed) > 0 {
			bps = append(bps, gen.LengthBreakpoint{Ver: ver, Entries: changed})
		}
		prev = cur
	}
	return bps, nil
}

// buildMapStocJoinPass walks SemanticDB receive-direction entries and for each
// one looks up the rathena_struct in the VersionTable. It emits LengthBreakpoints
// for each PACKETVER range where the struct has a known TotalSize. This covers
// Gap A (packets_struct.hpp structs) and Gap C (SYNTH_* synthetic structs).
func buildMapStocJoinPass(vt preprocess.VersionTable, mappings []semantics.PacketMapping) ([]gen.LengthBreakpoint, error) {
	// Collect breakpoints as a map[packetver]map[packetID]length to dedup.
	type lenTable map[uint16]int16
	bpMap := make(map[uint32]lenTable)

	for _, mapping := range mappings {
		if mapping.Direction != "receive" {
			continue
		}
		structName := mapping.RathenaStruct
		if structName == "" {
			continue
		}
		ranges, ok := vt[structName]
		if !ok {
			continue
		}
		id64, err := parseHexID(mapping.PacketID)
		if err != nil {
			log.Printf("  WARNING: join pass: cannot parse packet ID %q: %v", mapping.PacketID, err)
			continue
		}
		id := uint16(id64)

		for _, vr := range ranges {
			if vr.Layout == nil {
				continue
			}

			// Honour the SemanticDB packetver bounds on this mapping.
			// mapping.PacketverMin and PacketverMax constrain which VersionTable
			// ranges are valid for this (packet_id, struct) pairing.
			// A VersionTable range [vr.MinVer, vr.MaxVer) is relevant only if
			// it overlaps the mapping's [PacketverMin, PacketverMax) window.
			implMin := mapping.PacketverMin // 0 → 20030000 (default from loader)
			if implMin == 0 {
				implMin = 20030000
			}
			implMax := mapping.PacketverMax // 0 → no upper bound
			// vr.MaxVer==0 means "no upper bound" in VersionTable.
			vrMax := vr.MaxVer
			vrMin := vr.MinVer

			// Skip if the struct range ends at or before our impl window starts.
			if vrMax != 0 && vrMax <= uint32(implMin) {
				continue
			}
			// Skip if the struct range starts at or after our impl window ends.
			if implMax != 0 && vrMin >= uint32(implMax) {
				continue
			}
			size := vr.Layout.TotalSize
			var length int16
			if vr.Layout.Fields != nil {
				for _, f := range vr.Layout.Fields {
					if f.IsFlexArray {
						length = -1
						break
					}
				}
			}
			if length == 0 {
				if size <= 0 {
					continue
				}
				length = int16(size)
			}

			ver := vr.MinVer
			if ver == 20030000 {
				ver = 0
			}
			if bpMap[ver] == nil {
				bpMap[ver] = make(lenTable)
			}
			// Non-zero length always wins over a reset placeholder (0).
			if cur, exists := bpMap[ver][id]; !exists || (cur == 0 && length != 0) {
				bpMap[ver][id] = length
			}

			// If MaxVer is set, emit a "reset to 0" at that boundary so the
			// generated code stops applying this length from that version on —
			// but only if a non-zero length is not already set at that boundary
			// (i.e. a newer range for the same packet already claims that version).
			if vr.MaxVer != 0 {
				if bpMap[vr.MaxVer] == nil {
					bpMap[vr.MaxVer] = make(lenTable)
				}
				if existing, exists := bpMap[vr.MaxVer][id]; !exists || existing == 0 {
					// Only write reset if nothing better is registered yet.
					// The newer range will overwrite this 0 if it runs after us.
					bpMap[vr.MaxVer][id] = 0
				}
			}
		}
	}

	var bps []gen.LengthBreakpoint
	for ver, tbl := range bpMap {
		var entries []gen.LengthEntry
		for id, length := range tbl {
			entries = append(entries, gen.LengthEntry{ID: id, Length: length})
		}
		bps = append(bps, gen.LengthBreakpoint{Ver: ver, Entries: entries})
	}
	return bps, nil
}

// mergeBreakpoints merges two sorted breakpoint slices into one. Entries from
// both are combined; when two entries have the same (ver, id), the one from
// extra wins (it has higher specificity — struct-derived over clif_packetdb).
func mergeBreakpoints(base, extra []gen.LengthBreakpoint) []gen.LengthBreakpoint {
	// Build a map[ver]map[id]length from base.
	type lenTable map[uint16]int16
	bpMap := make(map[uint32]lenTable)
	for _, bp := range base {
		if bpMap[bp.Ver] == nil {
			bpMap[bp.Ver] = make(lenTable)
		}
		for _, e := range bp.Entries {
			bpMap[bp.Ver][e.ID] = e.Length
		}
	}
	// Overlay extra — extra wins on conflict.
	for _, bp := range extra {
		if bpMap[bp.Ver] == nil {
			bpMap[bp.Ver] = make(lenTable)
		}
		for _, e := range bp.Entries {
			bpMap[bp.Ver][e.ID] = e.Length
		}
	}
	// Flatten back to a slice.
	var result []gen.LengthBreakpoint
	for ver, tbl := range bpMap {
		var entries []gen.LengthEntry
		for id, length := range tbl {
			entries = append(entries, gen.LengthEntry{ID: id, Length: length})
		}
		result = append(result, gen.LengthBreakpoint{Ver: ver, Entries: entries})
	}
	return result
}

// mergeBreakpointsFillOnly merges extra into base but ONLY sets entries that
// are currently 0 (unknown) in base. Existing nonzero values (from
// clif_packetdb.hpp or packets.hpp HEADER_* scans) are never overridden.
// This is used for the S→C struct-size join pass (Part 3) to ensure that
// the ground-truth clif_packetdb lengths take priority over struct-derived
// sizes — particularly important for variable-length packets (length=-1)
// that happen to have a computable struct TotalSize.
func mergeBreakpointsFillOnly(base, extra []gen.LengthBreakpoint) []gen.LengthBreakpoint {
	// Build a base map for easy lookup: at any given (ver, id), what is the
	// cumulative length from Parts 1–2?
	type lenTable map[uint16]int16
	bpMap := make(map[uint32]lenTable)
	for _, bp := range base {
		if bpMap[bp.Ver] == nil {
			bpMap[bp.Ver] = make(lenTable)
		}
		for _, e := range bp.Entries {
			bpMap[bp.Ver][e.ID] = e.Length
		}
	}

	// Simulate the generated function's cumulative state to know whether a
	// packet already has a nonzero value at a given version.
	// Strategy: for each extra entry at (ver, id, length), check if the
	// base table has any entry for (id) at version <= ver that is nonzero.
	// If so, skip — clif_packetdb already owns this packet.

	// Collect all base versions sorted.
	var baseVers []uint32
	for v := range bpMap {
		baseVers = append(baseVers, v)
	}
	sort.Slice(baseVers, func(i, j int) bool { return baseVers[i] < baseVers[j] })

	// Build cumulative table: for each version threshold v, what is the last
	// known value for each packet ID from Parts 1–2?
	// We simulate: start at v=0, apply each block in order.
	cumulative := make(lenTable) // current state as of the last processed base ver
	// ver→snapshot of cumulative after applying that ver's base block
	snapshots := make(map[uint32]lenTable)
	// Precompute snapshots at each base version.
	snapshotKeys := make([]uint32, 0, len(baseVers)+1)
	snapMap := make(map[uint32]lenTable)
	cur := make(lenTable)
	snapMap[0] = make(lenTable)
	for _, v := range baseVers {
		for id, length := range bpMap[v] {
			cur[id] = length
		}
		snap := make(lenTable, len(cur))
		for k, vv := range cur {
			snap[k] = vv
		}
		snapMap[v] = snap
		snapshotKeys = append(snapshotKeys, v)
	}
	sort.Slice(snapshotKeys, func(i, j int) bool { return snapshotKeys[i] < snapshotKeys[j] })
	_ = cumulative
	_ = snapshots

	// For an extra entry at (extraVer, id, length): it's a "fill" — only apply
	// it if the simulated base state at extraVer has value 0 for that id.
	fillMap := make(map[uint32]lenTable)
	for _, bp := range extra {
		// Find the cumulative base state at bp.Ver.
		// Walk the snapshots up to and including bp.Ver.
		baseAtVer := make(lenTable)
		for _, sv := range snapshotKeys {
			if sv <= bp.Ver {
				for id, length := range bpMap[sv] {
					baseAtVer[id] = length
				}
			}
		}
		for _, e := range bp.Entries {
			if baseAtVer[e.ID] != 0 {
				// clif_packetdb or HEADER_* scan already has a nonzero value
				// at this version — do not override.
				continue
			}
			if fillMap[bp.Ver] == nil {
				fillMap[bp.Ver] = make(lenTable)
			}
			fillMap[bp.Ver][e.ID] = e.Length
		}
	}

	// Merge fillMap into bpMap.
	for ver, tbl := range fillMap {
		if bpMap[ver] == nil {
			bpMap[ver] = make(lenTable)
		}
		for id, length := range tbl {
			bpMap[ver][id] = length
		}
	}

	// Flatten back.
	var result []gen.LengthBreakpoint
	for ver, tbl := range bpMap {
		var entries []gen.LengthEntry
		for id, length := range tbl {
			entries = append(entries, gen.LengthEntry{ID: id, Length: length})
		}
		result = append(result, gen.LengthBreakpoint{Ver: ver, Entries: entries})
	}
	return result
}

func parseHexID(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	return strconv.ParseUint(s, 16, 16)
}

func genEvents(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
	files, err := gen.GenerateEventsDirFiles(db, vt)
	if err != nil {
		return err
	}
	eventsDir := filepath.Join(outDir, "pkg", "events")
	if err := cleanGeneratedDir(eventsDir); err != nil {
		return err
	}
	for filename, src := range files {
		if err := writeFile(filepath.Join(eventsDir, filename), src); err != nil {
			return err
		}
	}
	log.Printf("  → pkg/events/ (%d files)", len(files))
	return nil
}

func genSend(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
	files, err := gen.GenerateSendDirFiles(db, vt)
	if err != nil {
		return err
	}
	sendDir := filepath.Join(outDir, "pkg", "send")
	if err := cleanGeneratedDir(sendDir); err != nil {
		return err
	}
	for filename, src := range files {
		if err := writeFile(filepath.Join(sendDir, filename), src); err != nil {
			return err
		}
	}
	log.Printf("  → pkg/send/ (%d files)", len(files))
	return nil
}

func genDecode(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
	files, skipped, err := gen.GenerateDecodeDirFiles(db, vt)
	if err != nil {
		return err
	}
	decodeDir := filepath.Join(outDir, "pkg", "decode")
	if err := cleanGeneratedDir(decodeDir); err != nil {
		return err
	}
	for filename, src := range files {
		if err := writeFile(filepath.Join(decodeDir, filename), src); err != nil {
			return err
		}
	}
	log.Printf("  → pkg/decode/ (%d files, %d skipped)", len(files), len(skipped))
	if len(skipped) > 0 {
		log.Printf("  Skipped: %v", skipped[:min(5, len(skipped))])
	}
	return nil
}

func genEncode(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
	return genEncodeWithShuffleCheck(db, vt, outDir, nil)
}

// genEncodeWithShuffleCheck generates encode files and validates that no generated
// encoder hardcodes a packet ID that appears in the shuffle map.
// shuffleBaseIDs may be nil to skip the check (e.g. when clif_shuffle.hpp is unavailable).
func genEncodeWithShuffleCheck(db *semantics.DB, vt preprocess.VersionTable, outDir string, shuffleBaseIDs map[uint16]bool) error {
	encodeDir := filepath.Join(outDir, "pkg", "encode")

	// Build the allowlist from hand-written encoders BEFORE cleaning the directory.
	// Hand-written files (no "// Code generated" header) are already correct by
	// definition — their IDs should not trigger the shuffle overlap check even if
	// the ID appears in the shuffle map. This handles:
	//   - friends_add (0x0202): hand-written dispatcher using shuffledCtoSID
	//   - character_move (0x035F): hand-written with documented pv limitation
	//
	// Additional explicit exceptions:
	//   - homunculus_menu (0x022D): out-of-scope (homunculus/mercenary not supported)
	//   - master_login (0x0064): CA_ login-server packet; shares ID with map shuffle
	//     entries by coincidence (different server, never shuffled on login server)
	var allowlist map[uint16]bool
	if shuffleBaseIDs != nil {
		allowlist = buildHandWrittenAllowlist(encodeDir, db)
		// Explicit exceptions: generated files whose IDs are false-positive shuffle overlaps
		allowlist[0x022D] = true // homunculus_menu — homunculus/mercenary out of scope
		allowlist[0x0064] = true // master_login — CA_ login server, never map-shuffled
		log.Printf("  %d encoder IDs allowlisted from shuffle check", len(allowlist))
	}

	files, skipped, err := gen.GenerateEncodeDirFilesWithShuffleCheck(db, vt, shuffleBaseIDs, allowlist)
	if cleanErr := cleanGeneratedDir(encodeDir); cleanErr != nil {
		return cleanErr
	}
	for filename, src := range files {
		if writeErr := writeFile(filepath.Join(encodeDir, filename), src); writeErr != nil {
			return writeErr
		}
	}
	log.Printf("  → pkg/encode/ (%d files, %d skipped)", len(files), len(skipped))
	if err != nil {
		// Shuffle overlap violations: fail the codegen run.
		return fmt.Errorf("encoder shuffle overlap check failed — run was aborted to prevent shipping wrong wire IDs:\n%w", err)
	}
	return nil
}

// buildHandWrittenAllowlist scans the encode directory for hand-written encoder files
// (those without a "// Code generated" header) and returns the set of packet IDs
// they hardcode. These IDs are excluded from the shuffle overlap check because the
// hand-written files are already correct by definition.
func buildHandWrittenAllowlist(encodeDir string, db *semantics.DB) map[uint16]bool {
	allowlist := make(map[uint16]bool)
	entries, err := os.ReadDir(encodeDir)
	if err != nil {
		return allowlist
	}

	// Build action-name → packet IDs map from DB
	actionPacketIDs := make(map[string][]uint16)
	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			if v, err2 := strconv.ParseUint(strings.TrimPrefix(impl.PacketID, "0x"), 16, 16); err2 == nil {
				actionPacketIDs[name] = append(actionPacketIDs[name], uint16(v))
			}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(encodeDir, entry.Name())
		content, err2 := os.ReadFile(path)
		if err2 != nil {
			continue
		}
		// Skip generated files — only allowlist hand-written ones
		if strings.HasPrefix(string(content), "// Code generated") {
			continue
		}
		// Extract the action name from the filename (strip .go suffix)
		actionFile := strings.TrimSuffix(entry.Name(), ".go")
		for _, pid := range actionPacketIDs[actionFile] {
			allowlist[pid] = true
		}
		// Also extract any hardcoded 0xNNNN IDs directly from the file
		for _, m := range regexp.MustCompile(`0x([0-9A-Fa-f]{4})`).FindAllStringSubmatch(string(content), -1) {
			if v, err3 := strconv.ParseUint(m[1], 16, 16); err3 == nil {
				allowlist[uint16(v)] = true
			}
		}
	}
	return allowlist
}

// buildShuffleBaseIDs parses clif_shuffle.hpp and clif_packetdb.hpp to build the
// set of C→S base packet IDs that appear in any shuffle block. These are the IDs
// that any generated encoder must NOT hardcode (they must use shuffledCtoSID instead).
func buildShuffleBaseIDs(cfg preprocess.Config) (map[uint16]bool, error) {
	shuffleFile := filepath.Join(cfg.RathenaRoot, "src", "map", "clif_shuffle.hpp")
	content, err := os.ReadFile(shuffleFile)
	if err != nil {
		return nil, fmt.Errorf("read clif_shuffle.hpp: %w", err)
	}
	sections, err := preprocess.ParseShuffle(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse shuffle: %w", err)
	}
	packetdbPreprocessed, err := preprocess.Preprocess(cfg, preprocess.SourceClifPacketDB, 20180307)
	if err != nil {
		return nil, fmt.Errorf("preprocess packetdb: %w", err)
	}
	entries, err := preprocess.ParsePacketDB(packetdbPreprocessed)
	if err != nil {
		return nil, fmt.Errorf("parse packetdb: %w", err)
	}
	baseIDs := preprocess.HandlerBaseIDs(entries)
	breakpoints := gen.BuildShuffleBreakpoints(sections, baseIDs)

	result := make(map[uint16]bool)
	for _, bp := range breakpoints {
		for _, e := range bp.Entries {
			result[e.BaseID] = true
		}
	}
	return result, nil
}

func genActions(db *semantics.DB, outDir string) error {
	src, err := gen.GenerateActionsFile(db)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "pkg", "session", "actions.go"), src)
}

func genReceiveDispatch(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
	src, err := gen.GenerateReceiveDispatchFile(db, vt)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "pkg", "session", "receive_dispatch.go"), src)
}

func genRegister(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
	encodeDir := filepath.Join(outDir, "pkg", "encode")
	src, err := gen.GenerateRegisterFileWithDir(db, vt, encodeDir)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "pkg", "encode", "register.go"), src)
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	// Do not overwrite hand-written files (files that do not start with the
	// codegen header are assumed to be manually maintained).
	const generatedHeader = "// Code generated by internal/codegen. DO NOT EDIT."
	if existing, err := os.ReadFile(path); err == nil {
		if !strings.HasPrefix(string(existing), generatedHeader) {
			return nil // preserve hand-written file
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// cleanGeneratedDir removes all *.go files in dir that start with the codegen
// header ("// Code generated by internal/codegen. DO NOT EDIT.").
// This ensures stale files (e.g. renamed actions) are removed on each codegen run.
func cleanGeneratedDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // dir doesn't exist yet — nothing to clean
		}
		return fmt.Errorf("readdir %s: %w", dir, err)
	}
	const header = "// Code generated by internal/codegen. DO NOT EDIT."
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.HasPrefix(string(data), header) {
			if err := os.Remove(p); err != nil {
				return fmt.Errorf("remove %s: %w", p, err)
			}
			removed++
		}
	}
	if removed > 0 {
		log.Printf("  cleaned %d stale generated files from %s", removed, dir)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// deduplicateLengthBreakpoints removes entries from later breakpoints where the
// value matches what was already set by an earlier breakpoint. This prevents
// redundant assignments like "t[0x0284] = 14" appearing in multiple if-blocks
// when the value was already established in the baseline.
func deduplicateLengthBreakpoints(bps []gen.LengthBreakpoint) []gen.LengthBreakpoint {
	// Sort by version ascending (stable, so equal versions stay ordered)
	sort.Slice(bps, func(i, j int) bool { return bps[i].Ver < bps[j].Ver })

	state := make(map[uint16]int16)
	var result []gen.LengthBreakpoint
	for _, bp := range bps {
		var kept []gen.LengthEntry
		for _, e := range bp.Entries {
			if cur, ok := state[e.ID]; !ok || cur != e.Length {
				kept = append(kept, e)
				state[e.ID] = e.Length
			}
		}
		if len(kept) > 0 {
			result = append(result, gen.LengthBreakpoint{Ver: bp.Ver, Entries: kept})
		}
	}
	return result
}
