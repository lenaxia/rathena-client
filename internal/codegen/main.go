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
	"strings"

	"github.com/lenaxia/ragnarok-go-client/internal/codegen/gen"
	"github.com/lenaxia/ragnarok-go-client/internal/codegen/preprocess"
	"github.com/lenaxia/ragnarok-go-client/internal/codegen/semantics"
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

	// Step 1: Generate shuffle table (session/shuffle_map.go)
	log.Println("Generating shuffle table...")
	if err := genShuffle(cfg, outDir); err != nil {
		return fmt.Errorf("shuffle: %w", err)
	}
	log.Println("  → pkg/session/shuffle_map.go")

	// Step 2: Generate obfuscation keys (session/obfuscation_keys.go)
	log.Println("Generating obfuscation keys...")
	if err := genObfuscation(cfg, outDir); err != nil {
		return fmt.Errorf("obfuscation: %w", err)
	}
	log.Println("  → pkg/session/obfuscation_keys.go")

	// Step 3: Generate length tables (session/lengths_*.go)
	log.Println("Generating length tables...")
	if err := genLengths(cfg, outDir); err != nil {
		return fmt.Errorf("lengths: %w", err)
	}
	log.Println("  → pkg/session/lengths_login.go, lengths_char.go, lengths_map.go")

	// Step 4: Load semantic DB.
	log.Printf("Loading semantic DB from %s...", semanticsPath)
	db, err := semantics.LoadFile(semanticsPath)
	if err != nil {
		return fmt.Errorf("load semantics: %w", err)
	}
	log.Printf("  Loaded %d semantic actions", len(db.Actions))

	// Step 5: Build VersionTable from packets_struct.hpp.
	log.Println("Building VersionTable from packets_struct.hpp...")
	vt, err := buildVersionTable(cfg)
	if err != nil {
		return fmt.Errorf("build version table: %w", err)
	}
	log.Printf("  VersionTable has %d structs", len(vt))

	// Step 5b: Inject synthetic structs (SYNTH_*) for structless packets.
	log.Println("Injecting synthetic struct layouts...")
	if err := injectSynthetic(cfg, vt); err != nil {
		return fmt.Errorf("inject synthetic structs: %w", err)
	}
	log.Printf("  VersionTable now has %d structs (after synthetic injection)", len(vt))

	// Step 5c: Inject structs from common/packets.hpp (login/char server packets).
	// These are not in packets_struct.hpp so buildVersionTable never sees them.
	log.Println("Injecting common/packets.hpp struct layouts...")
	if err := injectCommonPacketStructs(cfg, vt); err != nil {
		return fmt.Errorf("inject common packet structs: %w", err)
	}
	log.Printf("  VersionTable now has %d structs (after common injection)", len(vt))

	// Step 6: Generate event structs (pkg/events/*.go)
	log.Println("Generating event structs...")
	if err := genEvents(db, outDir); err != nil {
		return fmt.Errorf("events: %w", err)
	}

	// Step 7: Generate send request types (pkg/send/*.go)
	log.Println("Generating send request types...")
	if err := genSend(db, outDir); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	// Step 8: Generate decode functions (pkg/decode/*.go)
	log.Println("Generating decode functions...")
	if err := genDecode(db, vt, outDir); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	// Step 9: Generate encode functions (pkg/encode/*.go)
	log.Println("Generating encode functions...")
	if err := genEncode(db, vt, outDir); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

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

// commonStructsToInject is the set of struct names from common/packets.hpp that
// should be injected into the VersionTable for decode codegen.
// Each struct name must exactly match the C struct name in that header.
var commonStructsToInject = []string{
	"PACKET_AC_ACCEPT_LOGIN",
}

// injectCommonPacketStructs processes common/packets.hpp at all its PACKETVER
// breakpoints and injects the structs listed in commonStructsToInject into the
// VersionTable. This is necessary because common/packets.hpp is not processed by
// buildVersionTable (which only reads packets_struct.hpp from the map server).
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

	injected := 0
	for _, structName := range commonStructsToInject {
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

	if injected == 0 {
		return fmt.Errorf("injectCommonPacketStructs: no structs injected (wanted: %v)", commonStructsToInject)
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

	return writeFile(filepath.Join(outDir, "pkg", "session", "shuffle_map.go"), src)
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

func genLengths(cfg preprocess.Config, outDir string) error {
	// --- Map server lengths (from clif_packetdb.hpp) ---
	mapBreakpoints, err := buildMapLengthBreakpoints(cfg)
	if err != nil {
		return fmt.Errorf("build map length breakpoints: %w", err)
	}

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

func genEvents(db *semantics.DB, outDir string) error {
	files, err := gen.GenerateEventsDirFiles(db)
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

func genSend(db *semantics.DB, outDir string) error {
	files, err := gen.GenerateSendDirFiles(db)
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
	files, skipped, err := gen.GenerateEncodeDirFiles(db, vt)
	if err != nil {
		return err
	}
	encodeDir := filepath.Join(outDir, "pkg", "encode")
	if err := cleanGeneratedDir(encodeDir); err != nil {
		return err
	}
	for filename, src := range files {
		if err := writeFile(filepath.Join(encodeDir, filename), src); err != nil {
			return err
		}
	}
	log.Printf("  → pkg/encode/ (%d files, %d skipped)", len(files), len(skipped))
	return nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
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
