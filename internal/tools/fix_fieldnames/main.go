// Package main implements a one-shot tool that corrects Category B field name
// case mismatches in the SemanticDB.
//
// Category B: the field_mapping expression uses a wrong-case rAthena field name
// (e.g. "packet.BodyState" when the rAthena struct field is "bodyState").
// The codegen does a case-sensitive lookup, so these produce silent skips in the
// generated decode functions.
//
// The tool uses struct-specific resolution: for each field_mapping expression, it
// looks up the specific rAthena struct named in that implementation and finds the
// correct canonical field name within that struct. This resolves collisions (e.g.
// both "AID" and "aid" exist across different structs) by context.
//
// Usage:
//
//	go run ./internal/tools/fix_fieldnames/main.go \
//	    --rathena /path/to/rathena \
//	    --semantics semantics/mappings.yaml
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

func main() {
	rathenaRoot := flag.String("rathena", os.Getenv("HOME")+"/personal/rathena", "path to rAthena source root")
	semanticsPath := flag.String("semantics", "semantics/mappings.yaml", "path to mappings.yaml")
	packetver := flag.String("packetver", "20181121", "PACKETVER to use for GCC field name extraction")
	dryRun := flag.Bool("dry-run", false, "print corrections without writing")
	flag.Parse()

	log.Printf("fix_fieldnames: rathena=%s packetver=%s dry-run=%v", *rathenaRoot, *packetver, *dryRun)

	// Step 1: build per-struct field name map from GCC output.
	structFields, err := buildStructFieldMap(*rathenaRoot, *packetver)
	if err != nil {
		log.Fatalf("build struct field map: %v", err)
	}
	log.Printf("extracted %d structs from packets_struct.hpp", len(structFields))

	// Step 2: read mappings.yaml.
	raw, err := os.ReadFile(*semanticsPath)
	if err != nil {
		log.Fatalf("read semantics: %v", err)
	}

	// Step 3: apply struct-specific corrections.
	corrected, count, err := applyCorrections(string(raw), structFields, *dryRun)
	if err != nil {
		log.Fatalf("apply corrections: %v", err)
	}

	if *dryRun {
		log.Printf("dry-run: would apply %d corrections", count)
		return
	}

	// Step 4: write back.
	if err := os.WriteFile(*semanticsPath, []byte(corrected), 0644); err != nil {
		log.Fatalf("write semantics: %v", err)
	}
	log.Printf("done: applied %d corrections to %s", count, *semanticsPath)
}

// structFieldMap maps struct_name → (lowercase_field_name → canonical_field_name).
type structFieldMap map[string]map[string]string

// buildStructFieldMap runs g++ -E -P on packets_struct.hpp and builds a per-struct
// map of lowercase field name → canonical field name.
func buildStructFieldMap(rathenaRoot, packetver string) (structFieldMap, error) {
	header := rathenaRoot + "/src/map/packets_struct.hpp"
	cmd := exec.Command("g++", "-E", "-P",
		"-DPACKETVER="+packetver,
		"-DPACKETVER_MAIN_NUM="+packetver,
		"-DPACKETVER_RE_NUM=0",
		"-DPACKETVER_ZERO_NUM=0",
		"-I", rathenaRoot+"/src",
		header,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("g++ preprocess: %w", err)
	}

	reStructStart := regexp.MustCompile(`^\s*struct\s+(\w+)\s*\{`)
	reField := regexp.MustCompile(`^\s*(?:struct\s+)?(\w+)\s+(\w+)(?:\[[^\]]*\])?\s*;`)

	result := make(structFieldMap)
	var currentStruct string
	depth := 0

	for _, line := range strings.Split(string(out), "\n") {
		if m := reStructStart.FindStringSubmatch(line); m != nil {
			currentStruct = m[1]
			result[currentStruct] = make(map[string]string)
			depth = 1
			continue
		}
		if currentStruct != "" {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				currentStruct = ""
				continue
			}
			if m := reField.FindStringSubmatch(line); m != nil {
				name := m[2]
				result[currentStruct][strings.ToLower(name)] = name
			}
		}
	}
	return result, nil
}

// rePacketField matches "packet.FIELDNAME" in a field_mapping expression.
var rePacketField = regexp.MustCompile(`packet\.([A-Za-z_][A-Za-z0-9_]*)`)

// reImplBlock matches the beginning of an implementations block entry so we can
// track which struct_name is in scope while scanning the YAML line by line.
// We parse the YAML as plain text (line by line) to preserve formatting exactly.
var reStructName = regexp.MustCompile(`^\s+struct_name:\s+(\S+)`)

// applyCorrections scans the YAML text line by line, tracks the current
// struct_name context, and replaces wrong-case field references in field_mapping
// values with the correct canonical names from the struct-specific field map.
func applyCorrections(content string, sfm structFieldMap, dryRun bool) (string, int, error) {
	lines := strings.Split(content, "\n")
	count := 0

	type correctionRecord struct {
		wrong   string
		correct string
	}
	summaryMap := make(map[string]*correctionRecord)

	// We track the most recently seen struct_name in the YAML.
	// The field_mapping entries always follow struct_name in the same impl block.
	currentStruct := ""

	for i, line := range lines {
		// Track struct_name context.
		if m := reStructName.FindStringSubmatch(line); m != nil {
			currentStruct = m[1]
			continue
		}

		// Only process lines containing "packet." references.
		if !strings.Contains(line, "packet.") || currentStruct == "" {
			continue
		}

		// Look up the field map for the current struct.
		fieldMap, ok := sfm[currentStruct]
		if !ok {
			// Struct not in packets_struct.hpp (e.g. packets.hpp struct) — skip.
			continue
		}

		newLine := rePacketField.ReplaceAllStringFunc(line, func(match string) string {
			fieldName := match[len("packet."):]
			lower := strings.ToLower(fieldName)

			correct, exists := fieldMap[lower]
			if !exists {
				// Field not in this struct at all (Category C or wrong struct) — leave.
				return match
			}
			if correct == fieldName {
				// Already the correct case — leave.
				return match
			}

			// Wrong case — replace.
			count++
			key := fieldName + "→" + correct
			if _, seen := summaryMap[key]; !seen {
				summaryMap[key] = &correctionRecord{wrong: fieldName, correct: correct}
			}
			return "packet." + correct
		})

		lines[i] = newLine
	}

	// Print summary.
	if len(summaryMap) > 0 {
		keys := make([]string, 0, len(summaryMap))
		for k := range summaryMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("Corrections (%d total replacements, %d unique field renames):\n",
			count, len(summaryMap))
		for _, k := range keys {
			r := summaryMap[k]
			fmt.Printf("  packet.%-28s → packet.%s\n", r.wrong, r.correct)
		}
	}

	return strings.Join(lines, "\n"), count, nil
}
