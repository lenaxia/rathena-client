package preprocess

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Config holds the paths and options for preprocessing rAthena headers.
type Config struct {
	RathenaRoot    string // path to rAthena repo root (RATHENA_ROOT)
	PacketsHPPStub string // path to packets_hpp_stub.h
	CommonHPPStub  string // path to common_hpp_stub.h
	SyntheticHPP   string // path to synthetic_structs.hpp (optional; used by InjectSyntheticStructs)
}

// Source identifies which rAthena header to preprocess.
type Source int

const (
	SourcePacketsStruct   Source = iota // src/map/packets_struct.hpp
	SourcePackets                       // src/map/packets.hpp (requires stubs)
	SourceCommonPackets                 // src/common/packets.hpp (requires stubs)
	SourceClifPacketDB                  // src/map/clif_packetdb.hpp
	SourceClifShuffle                   // src/map/clif_shuffle.hpp
	SourceClifObfuscation               // src/map/clif_obfuscation.hpp (needs -DPACKET_OBFUSCATION)
	SourceSynthetic                     // internal/codegen/stubs/synthetic_structs.hpp
)

// Preprocess runs g++ -E -P on the given source at the given PACKETVER.
// Returns the flat preprocessed output with all #if conditionals resolved.
func Preprocess(cfg Config, src Source, packetver uint32) (string, error) {
	pv := fmt.Sprintf("%d", packetver)
	args := []string{
		"-E", "-P",
		fmt.Sprintf("-DPACKETVER=%s", pv),
		fmt.Sprintf("-DPACKETVER_MAIN_NUM=%s", pv),
		"-I", cfg.RathenaRoot + "/src",
		"-I", cfg.RathenaRoot + "/src/map",
		"-I", cfg.RathenaRoot + "/src/common",
	}

	switch src {
	case SourcePacketsStruct:
		// packets_struct.hpp uses macros defined in packets.hpp / map.hpp
		// (e.g. MAX_ITEM_OPTIONS, MESSAGE_SIZE) — include the same stub as
		// SourcePackets so those macros resolve and arrays size correctly.
		// Without this, e.g. `ItemOptions option_data[MAX_ITEM_OPTIONS]`
		// parses as a flex-array (size 0), which mis-aligns downstream
		// fields in structs like PACKET_ZC_ADD_EXCHANGE_ITEM.
		if cfg.PacketsHPPStub != "" {
			args = append(args, "-include"+cfg.PacketsHPPStub)
		}
		args = append(args, cfg.RathenaRoot+"/src/map/packets_struct.hpp")
	case SourcePackets:
		args = append(args, "-include"+cfg.PacketsHPPStub)
		args = append(args, cfg.RathenaRoot+"/src/map/packets.hpp")
	case SourceCommonPackets:
		args = append(args, "-I"+cfg.RathenaRoot+"/src/common")
		args = append(args, "-include"+cfg.CommonHPPStub)
		args = append(args, cfg.RathenaRoot+"/src/common/packets.hpp")
	case SourceClifPacketDB:
		args = append(args, cfg.RathenaRoot+"/src/map/clif_packetdb.hpp")
	case SourceClifShuffle:
		args = append(args, cfg.RathenaRoot+"/src/map/clif_shuffle.hpp")
	case SourceClifObfuscation:
		args = append(args, "-DPACKET_OBFUSCATION")
		args = append(args, cfg.RathenaRoot+"/src/map/clif_obfuscation.hpp")
	case SourceSynthetic:
		if cfg.SyntheticHPP == "" {
			return "", fmt.Errorf("SourceSynthetic: SyntheticHPP path not set in Config")
		}
		// synthetic_structs.hpp only depends on stdint.h — no rAthena includes needed.
		// Override args to use only the standard include path.
		args = []string{"-E", "-P", "-x", "c++", cfg.SyntheticHPP}
	default:
		return "", fmt.Errorf("unknown source %d", src)
	}

	cmd := exec.Command("g++", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("g++ failed for %v at %d: %w\nstderr: %s", src, packetver, err, stderr.String())
	}
	return stdout.String(), nil
}

// InjectSyntheticStructs preprocesses synthetic_structs.hpp and injects each
// SYNTH_* struct into the VersionTable as a static (single-version) entry.
//
// Synthetic structs never change across PACKETVER — they model structless packets
// with fixed layouts derived from clif_packetdb.hpp parseable_packet() entries.
// Each is injected with MinVer=20030000, MaxVer=0 (covers all versions).
//
// The cfg.SyntheticHPP field must be set to the path of synthetic_structs.hpp.
// Returns an error if preprocessing fails; individual struct parse failures are
// logged (as warnings) and skipped rather than aborting.
func InjectSyntheticStructs(cfg Config, vt VersionTable) error {
	if cfg.SyntheticHPP == "" {
		return fmt.Errorf("InjectSyntheticStructs: SyntheticHPP path not set in Config")
	}

	preprocessed, err := Preprocess(cfg, SourceSynthetic, 0)
	if err != nil {
		return fmt.Errorf("preprocess synthetic_structs.hpp: %w", err)
	}

	// Use a representative packetver for type-size resolution (all SYNTH_ types are
	// fixed and don't vary by version; any value works).
	db, err := ExtractStructs(preprocessed, 20181002)
	if err != nil {
		return fmt.Errorf("extract synthetic structs: %w", err)
	}

	injected := 0
	for name, layout := range db {
		// Only inject SYNTH_* names — skip any incidental system structs pulled
		// in by stdint.h on some platforms.
		if !strings.HasPrefix(name, "SYNTH_") {
			continue
		}
		if layout == nil {
			continue
		}
		l := *layout // copy
		vt[name] = []VersionedLayout{
			{
				MinVer: 20030000,
				MaxVer: 0, // no upper bound — valid for all versions
				Layout: &l,
			},
		}
		injected++
	}

	if injected == 0 {
		return fmt.Errorf("InjectSyntheticStructs: no SYNTH_ structs found in %s", cfg.SyntheticHPP)
	}

	return nil
}

// reStructBody matches "struct NAME {" in flat preprocessed output.
// After -E -P, the output has no #if lines — just resolved content.
var reStructStart = regexp.MustCompile(`^struct\s+(\w+)\s*\{`)

// ExtractStructs scans preprocessed output and extracts all struct bodies.
// Returns a StructDB mapping struct names to their parsed layouts.
func ExtractStructs(preprocessed string, packetver uint32) (StructDB, error) {
	db := make(StructDB)
	lines := strings.Split(preprocessed, "\n")

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		m := reStructStart.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		structName := m[1]

		// Collect body until matching '}'.
		// rAthena structs are flat after preprocessing (no nested struct in body,
		// but some may have trailing __attribute__((packed)) after the closing '}'.
		depth := 0
		var bodyLines []string
		start := i
		for i < len(lines) {
			l := lines[i]
			depth += strings.Count(l, "{") - strings.Count(l, "}")
			if i > start {
				// Don't include the opening "struct NAME {" line itself.
				bodyLines = append(bodyLines, l)
			}
			if depth == 0 && i > start {
				break
			}
			i++
		}
		i++ // advance past closing '}'

		// Remove the closing '}' line and any trailing attribute/semicolon.
		for len(bodyLines) > 0 {
			last := strings.TrimSpace(bodyLines[len(bodyLines)-1])
			if last == "" || strings.HasPrefix(last, "}") || strings.HasPrefix(last, "__attribute__") {
				bodyLines = bodyLines[:len(bodyLines)-1]
			} else {
				break
			}
		}

		body := strings.Join(bodyLines, "\n")
		// Pass the already-parsed structs so nested struct field sizes can be resolved.
		layout, err := ParseStructBody(body, structName, packetver, db)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", structName, err)
		}
		db[structName] = layout
	}

	return db, nil
}
