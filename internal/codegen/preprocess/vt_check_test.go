package preprocess

import (
	"fmt"
	"os"
	"testing"
)

// TestCheckSendStructs verifies that all struct names used by FSM-owned semantic
// actions are resolvable from the VersionTable (either via synthetic_structs.hpp
// injection or via common/packets.hpp injection).
//
// All entries must print FOUND — any MISSING entry means codegen will produce an
// empty send struct and a broken (non-dispatching) encode function.
//
// This test requires the rAthena source tree at the hardcoded path and g++ on PATH.
// It is skipped automatically in CI where neither is available.
func TestCheckSendStructs(t *testing.T) {
	rathenaRoot := "/home/mikekao/personal/rathena"

	// Skip if rAthena source tree is not present (e.g. CI).
	if _, err := os.Stat(rathenaRoot); os.IsNotExist(err) {
		t.Skipf("rAthena source not found at %s — skipping (local-only test)", rathenaRoot)
	}
	synthPath := "/home/mikekao/personal/rathena-client/internal/codegen/stubs/synthetic_structs.hpp"
	pv := uint32(20200401)

	cfg := Config{
		RathenaRoot:   rathenaRoot,
		SyntheticHPP:  synthPath,
		CommonHPPStub: "/home/mikekao/personal/rathena-client/internal/codegen/stubs/common_hpp_stub.h",
	}

	// Build base VersionTable from packets_struct.hpp (map server structs).
	preprocessed, err := Preprocess(cfg, SourcePacketsStruct, pv)
	if err != nil {
		t.Fatal(err)
	}
	db, err := ExtractStructs(preprocessed, pv)
	if err != nil {
		t.Fatal(err)
	}
	vt := make(VersionTable)
	for name, layout := range db {
		if layout != nil {
			vt[name] = []VersionedLayout{{MinVer: 20030000, Layout: layout}}
		}
	}

	// Inject synthetic structs (SYNTH_CZ_*, SYNTH_CH_*, etc.)
	if err := InjectSyntheticStructs(cfg, vt); err != nil {
		t.Fatalf("InjectSyntheticStructs: %v", err)
	}

	// Inject common/packets.hpp structs (PACKET_CA_*, PACKET_CH_*, PACKET_AC_*, etc.)
	commonPreprocessed, err := Preprocess(cfg, SourceCommonPackets, pv)
	if err != nil {
		t.Fatalf("Preprocess common/packets.hpp: %v", err)
	}
	commonDB, err := ExtractStructs(commonPreprocessed, pv)
	if err != nil {
		t.Fatalf("ExtractStructs common: %v", err)
	}
	for name, layout := range commonDB {
		if layout != nil && layout.Available {
			vt[name] = []VersionedLayout{{MinVer: 20030000, Layout: layout}}
		}
	}

	// All struct names used by FSM semantic action implementations.
	// SYNTH_* = structless in rAthena, defined in synthetic_structs.hpp
	// PACKET_* = real rAthena struct from common/packets.hpp or packets_struct.hpp
	//
	// SYNTH_CH_ENTER (0x0275) is intentionally excluded — it has UNAVAILABLE_STRUCT
	// and its layout is unknown. The game_login 0x0275 impl is expected to be skipped
	// by codegen gracefully.
	targets := []string{
		// master_login (0x0064 CA_LOGIN)
		"PACKET_CA_LOGIN",
		// game_login (0x0065 CH_ENTER old format)
		"SYNTH_CH_ENTER_0x0065",
		// char_login / select_character (0x0066 CH_SELECT_CHAR)
		"PACKET_CH_SELECT_CHAR",
		// request_character_page (0x09A1 CH_CHARLIST_REQ)
		"PACKET_CH_CHARLIST_REQ",
		// map_login (0x0436 CZ_ENTER2)
		"SYNTH_CZ_ENTER",
		// map_loaded (0x007D CZ_NOTIFY_ACTORINIT)
		"SYNTH_CZ_NOTIFY_ACTORINIT",
		// time_sync_response (0x007E CZ_REQUEST_TIME)
		"SYNTH_CZ_REQUEST_TIME",
		// time_sync_response (0x0360 CZ_REQUEST_TIME2)
		"SYNTH_CZ_REQUEST_TIME2",
	}

	allFound := true
	for _, name := range targets {
		ranges, ok := vt[name]
		if ok && len(ranges) > 0 && ranges[0].Layout != nil && ranges[0].Layout.Available {
			fmt.Printf("FOUND    %s: %d bytes, %d fields\n", name, ranges[0].Layout.TotalSize, len(ranges[0].Layout.Fields))
		} else {
			fmt.Printf("MISSING  %s\n", name)
			allFound = false
		}
	}

	if !allFound {
		t.Error("one or more structs are missing from the VersionTable — codegen will produce broken encode functions")
	}
}
