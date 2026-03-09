package preprocess

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
)

// reDateCondition matches PACKETVER comparison conditions of the form:
//
//	PACKETVER >= 20180307
//	PACKETVER_MAIN_NUM <= 20221005
//	PACKETVER == 20190220
//	PACKETVER > 20180000
var reDateCondition = regexp.MustCompile(`#(?:if|elif)\s+PACKETVER(?:_[A-Z_]+)?\s*[<>=!]+\s*(\d{8})`)

// ExtractBreakpointsFromFile scans a rAthena header file for PACKETVER comparison
// conditions and returns the unique set of date integers (e.g. 20180307).
// The file is scanned as raw text — no preprocessing required.
func ExtractBreakpointsFromFile(path string) ([]uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[uint32]bool)
	scanner := bufio.NewScanner(f)
	// Some rAthena lines are long — increase buffer.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		for _, m := range reDateCondition.FindAllStringSubmatch(scanner.Text(), -1) {
			if v, err := strconv.ParseUint(m[1], 10, 32); err == nil {
				seen[uint32(v)] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := make([]uint32, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	return out, nil
}

// AllBreakpoints returns the union of PACKETVER breakpoints across all three
// rAthena headers, sorted ascending. Each returned value is a date at which
// at least one struct changes.
//
// The caller should also include packetver=1 (the "pre-all" baseline) in any
// preprocessing run to capture structs that exist from the very beginning.
func AllBreakpoints(cfg Config) ([]uint32, error) {
	files := []string{
		cfg.RathenaRoot + "/src/map/packets_struct.hpp",
		cfg.RathenaRoot + "/src/map/packets.hpp",
		cfg.RathenaRoot + "/src/common/packets.hpp",
		cfg.RathenaRoot + "/src/map/clif_packetdb.hpp",
		cfg.RathenaRoot + "/src/map/clif_shuffle.hpp",
	}

	var all []uint32
	seen := make(map[uint32]bool)
	for _, f := range files {
		dates, err := ExtractBreakpointsFromFile(f)
		if err != nil {
			return nil, err
		}
		for _, d := range dates {
			if !seen[d] {
				seen[d] = true
				all = append(all, d)
			}
		}
	}
	return SortBreakpoints(all), nil
}
