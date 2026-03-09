//go:build integration

package preprocess_test

import (
	"github.com/lenaxia/ragnarok-go-client/internal/codegen/preprocess"
	"os"
	"testing"
)

func TestExtractStructs_PacketIdleUnit_Live(t *testing.T) {
	home := os.Getenv("HOME")
	cfg := preprocess.Config{
		RathenaRoot:    home + "/personal/rathena",
		PacketsHPPStub: home + "/personal/rathena-client/internal/codegen/stubs/packets_hpp_stub.h",
		CommonHPPStub:  home + "/personal/rathena-client/internal/codegen/stubs/common_hpp_stub.h",
	}

	out, err := preprocess.Preprocess(cfg, preprocess.SourcePacketsStruct, 20181121)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}

	db, err := preprocess.ExtractStructs(out, 20181121)
	if err != nil {
		t.Fatalf("ExtractStructs: %v", err)
	}

	layout, ok := db["packet_idle_unit"]
	if !ok {
		t.Fatal("packet_idle_unit not found")
	}
	if layout.TotalSize != 108 {
		t.Errorf("TotalSize = %d, want 108", layout.TotalSize)
	}

	byName := make(map[string]*preprocess.Field)
	for i := range layout.Fields {
		f := &layout.Fields[i]
		byName[f.Name] = f
	}

	checks := []struct {
		name         string
		offset, size int
	}{
		{"weapon", 27, 4},
		{"shield", 31, 4},
		{"accessory", 35, 2},
		{"PosDir", 63, 3},
		{"name", 84, 24},
	}
	for _, c := range checks {
		f, ok := byName[c.name]
		if !ok {
			t.Errorf("field %q not found", c.name)
			continue
		}
		if f.Offset != c.offset || f.Size != c.size {
			t.Errorf("%s: offset=%d size=%d, want offset=%d size=%d", c.name, f.Offset, f.Size, c.offset, c.size)
		}
	}
}

func TestParsePacketDB_Live(t *testing.T) {
	home := os.Getenv("HOME")
	cfg := preprocess.Config{
		RathenaRoot: home + "/personal/rathena",
	}
	// Preprocess at a known PACKETVER to resolve all conditionals.
	preprocessed, err := preprocess.Preprocess(cfg, preprocess.SourceClifPacketDB, 20180307)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	entries, err := preprocess.ParsePacketDB(preprocessed)
	if err != nil {
		t.Fatalf("ParsePacketDB: %v", err)
	}
	t.Logf("Parsed %d entries from clif_packetdb.hpp at 20180307", len(entries))

	// Use HandlerBaseIDs to find the canonical (first) assignment for each handler.
	baseIDs := preprocess.HandlerBaseIDs(entries)

	// clif_parse_WalkToXY base ID should be 0x0085 (first occurrence)
	if e, ok := baseIDs["clif_parse_WalkToXY"]; !ok {
		t.Error("clif_parse_WalkToXY not found")
	} else if e.ID != 0x0085 {
		t.Errorf("clif_parse_WalkToXY base ID = 0x%04X, want 0x0085", e.ID)
	}

	// clif_parse_TickSend base ID should be 0x007e
	if e, ok := baseIDs["clif_parse_TickSend"]; !ok {
		t.Error("clif_parse_TickSend not found")
	} else if e.ID != 0x007e {
		t.Errorf("clif_parse_TickSend base ID = 0x%04X, want 0x007e", e.ID)
	}

	// clif_parse_WantToConnection base ID: first occurrence in old-style
	// clif_packetdb.hpp is 0x0072 (pre-20040705). The modern 0x0436 ID is assigned
	// later in the file and appears in clif_shuffle.hpp sections as well.
	if e, ok := baseIDs["clif_parse_WantToConnection"]; !ok {
		t.Error("clif_parse_WantToConnection not found")
	} else if e.ID != 0x0072 && e.ID != 0x0436 {
		t.Errorf("clif_parse_WantToConnection base ID = 0x%04X, want 0x0072 or 0x0436", e.ID)
	}
}

func TestParseShuffle_Live(t *testing.T) {
	home := os.Getenv("HOME")
	content, err := os.ReadFile(home + "/personal/rathena/src/map/clif_shuffle.hpp")
	if err != nil {
		t.Fatalf("read clif_shuffle.hpp: %v", err)
	}
	sections, err := preprocess.ParseShuffle(string(content))
	if err != nil {
		t.Fatalf("ParseShuffle: %v", err)
	}
	t.Logf("Parsed %d shuffle sections from clif_shuffle.hpp", len(sections))
	if len(sections) < 100 {
		t.Errorf("expected >= 100 sections, got %d", len(sections))
	}

	// Find 20130515 section and verify WalkToXY is 0x0437
	for _, sec := range sections {
		if sec.PacketVer != 20130515 {
			continue
		}
		for _, e := range sec.Entries {
			if e.Handler == "clif_parse_WalkToXY" {
				if e.ID != 0x0437 {
					t.Errorf("20130515 WalkToXY = 0x%04X, want 0x0437", e.ID)
				}
				return
			}
		}
		t.Error("clif_parse_WalkToXY not found in 20130515")
		return
	}
	t.Error("section 20130515 not found")
}
