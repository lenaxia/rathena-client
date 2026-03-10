//go:build integration

package preprocess_test

import (
	"os"
	"testing"
	"github.com/lenaxia/rathena-client/internal/codegen/preprocess"
)

// TestBuildVersionTable_PacketIdleUnit tests the version table builder against
// the known breakpoints for packet_idle_unit. We verify that the correct total
// sizes are produced at each known breakpoint.
func TestBuildVersionTable_PacketIdleUnit(t *testing.T) {
	home := os.Getenv("HOME")
	cfg := preprocess.Config{
		RathenaRoot: home + "/personal/rathena",
	}

	// Known breakpoints for packet_idle_unit (from gate script section 6).
	breakpoints := []uint32{
		20080101, 20080102, 20091103, 20101124,
		20120221, 20131223, 20150513, 20181121,
	}

	var vdbs []preprocess.VersionedDB
	for _, pv := range breakpoints {
		out, err := preprocess.Preprocess(cfg, preprocess.SourcePacketsStruct, pv)
		if err != nil {
			t.Fatalf("Preprocess at %d: %v", pv, err)
		}
		db, err := preprocess.ExtractStructs(out, pv)
		if err != nil {
			t.Fatalf("ExtractStructs at %d: %v", pv, err)
		}
		vdbs = append(vdbs, preprocess.VersionedDB{Ver: pv, DB: db})
	}

	vt, err := preprocess.BuildVersionTable(vdbs)
	if err != nil {
		t.Fatalf("BuildVersionTable: %v", err)
	}

	// Verify total sizes at each breakpoint.
	wantSizes := map[uint32]int{
		20080101: 56,
		20080102: 60,
		20091103: 63,
		20101124: 65,
		20120221: 74,
		20131223: 102,
		20150513: 104,
		20181121: 108,
	}

	for pv, wantSize := range wantSizes {
		layout := vt.LayoutAt("packet_idle_unit", pv)
		if layout == nil {
			t.Errorf("pv=%d: layout is nil", pv)
			continue
		}
		if !layout.Available {
			t.Errorf("pv=%d: layout is UNAVAILABLE", pv)
			continue
		}
		if layout.TotalSize != wantSize {
			t.Errorf("pv=%d: TotalSize=%d, want %d", pv, layout.TotalSize, wantSize)
		}
	}
}

// TestBuildVersionTable_UnavailableStruct tests the tombstone pattern with
// packet_idle_unit2, which is UNAVAILABLE at PACKETVER >= 20091103.
func TestBuildVersionTable_UnavailableStruct(t *testing.T) {
	home := os.Getenv("HOME")
	cfg := preprocess.Config{
		RathenaRoot: home + "/personal/rathena",
	}

	breakpoints := []uint32{20091102, 20091103}
	var vdbs []preprocess.VersionedDB
	for _, pv := range breakpoints {
		out, err := preprocess.Preprocess(cfg, preprocess.SourcePacketsStruct, pv)
		if err != nil {
			t.Fatalf("Preprocess at %d: %v", pv, err)
		}
		db, err := preprocess.ExtractStructs(out, pv)
		if err != nil {
			t.Fatalf("ExtractStructs at %d: %v", pv, err)
		}
		vdbs = append(vdbs, preprocess.VersionedDB{Ver: pv, DB: db})
	}

	vt, err := preprocess.BuildVersionTable(vdbs)
	if err != nil {
		t.Fatalf("BuildVersionTable: %v", err)
	}

	// Before tombstone: should be available.
	pre := vt.LayoutAt("packet_idle_unit2", 20091102)
	if pre == nil {
		t.Fatal("pre-20091103: layout is nil, want available")
	}
	if !pre.Available {
		t.Fatal("pre-20091103: layout should be available")
	}

	// At tombstone: should be UNAVAILABLE.
	tomb := vt.LayoutAt("packet_idle_unit2", 20091103)
	if tomb == nil {
		t.Fatal("20091103: layout is nil, want UNAVAILABLE layout")
	}
	if tomb.Available {
		t.Fatal("20091103: layout should be UNAVAILABLE (tombstoned)")
	}
}
