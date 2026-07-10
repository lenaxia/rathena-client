package gen

import (
	"testing"

	"github.com/lenaxia/rathena-client/internal/codegen/preprocess"
)

// TestResolveLayout_PicksNewestForUnboundedImpl guards against the
// pre-v0.9.1 regression where an impl with PacketverMin=0 (unbounded)
// caused resolveLayout to fall back to the OLDEST available layout.
//
// Concrete motivating case: PACKET_CZ_REQ_GUILD_EMBLEM_IMG2 has two
// version ranges in rAthena packets_struct.hpp:5788-5803 — a 10-byte
// variant at pv 20190227..20190618 and a 14-byte variant at pv >=
// 20190619 (which adds a trailing `unused` uint32). The semantic DB
// records a single unbounded implementation with min=0. Modern rAthena
// builds emit the 14-byte variant, but the pre-fix resolveLayout
// picked the 10-byte layout — producing an encoder that sends 4 bytes
// short at goKore's target pv=20200401.
func TestResolveLayout_PicksNewestForUnboundedImpl(t *testing.T) {
	oldLayout := &preprocess.StructLayout{
		TotalSize: 10,
		Available: true,
	}
	newLayout := &preprocess.StructLayout{
		TotalSize: 14,
		Available: true,
	}
	vt := preprocess.VersionTable{
		"PACKET_CZ_REQ_GUILD_EMBLEM_IMG2": []preprocess.VersionedLayout{
			{MinVer: 20190227, MaxVer: 20190619, Layout: oldLayout},
			{MinVer: 20190619, MaxVer: 0, Layout: newLayout},
		},
	}
	got := resolveLayout("PACKET_CZ_REQ_GUILD_EMBLEM_IMG2", 0, vt)
	if got == nil {
		t.Fatal("expected a layout, got nil")
	}
	if got.TotalSize != 14 {
		t.Errorf("unbounded impl picked wrong layout: got size %d, want 14 (newest)", got.TotalSize)
	}
}

func TestResolveLayout_HonorsSpecificPacketverMin(t *testing.T) {
	oldLayout := &preprocess.StructLayout{TotalSize: 10, Available: true}
	newLayout := &preprocess.StructLayout{TotalSize: 14, Available: true}
	vt := preprocess.VersionTable{
		"PACKET_CZ_REQ_GUILD_EMBLEM_IMG2": []preprocess.VersionedLayout{
			{MinVer: 20190227, MaxVer: 20190619, Layout: oldLayout},
			{MinVer: 20190619, MaxVer: 0, Layout: newLayout},
		},
	}
	// An impl anchored to the old range should still get the old layout.
	got := resolveLayout("PACKET_CZ_REQ_GUILD_EMBLEM_IMG2", 20190500, vt)
	if got == nil || got.TotalSize != 10 {
		t.Errorf("packetverMin=20190500 should pick old layout (size 10), got %v", got)
	}
	// An impl at the new range's boundary should get the new layout.
	got = resolveLayout("PACKET_CZ_REQ_GUILD_EMBLEM_IMG2", 20190619, vt)
	if got == nil || got.TotalSize != 14 {
		t.Errorf("packetverMin=20190619 should pick new layout (size 14), got %v", got)
	}
	// An impl at the boundary between them should get the new layout too.
	got = resolveLayout("PACKET_CZ_REQ_GUILD_EMBLEM_IMG2", 20200401, vt)
	if got == nil || got.TotalSize != 14 {
		t.Errorf("packetverMin=20200401 should pick new layout (size 14), got %v", got)
	}
}

func TestResolveLayout_PacketverMinBelowAllRanges(t *testing.T) {
	// If packetverMin is set but lower than any range's MinVer, no range
	// "contains" it. Falling through should pick the newest.
	oldLayout := &preprocess.StructLayout{TotalSize: 10, Available: true}
	newLayout := &preprocess.StructLayout{TotalSize: 14, Available: true}
	vt := preprocess.VersionTable{
		"PACKET_TEST": []preprocess.VersionedLayout{
			{MinVer: 20190227, MaxVer: 20190619, Layout: oldLayout},
			{MinVer: 20190619, MaxVer: 0, Layout: newLayout},
		},
	}
	got := resolveLayout("PACKET_TEST", 20180101, vt)
	if got == nil || got.TotalSize != 14 {
		t.Errorf("packetverMin below all ranges should fall through to newest, got %v", got)
	}
}

func TestResolveLayout_MissingStruct(t *testing.T) {
	vt := preprocess.VersionTable{}
	got := resolveLayout("NONEXISTENT", 0, vt)
	if got != nil {
		t.Errorf("missing struct should return nil, got %v", got)
	}
}

func TestResolveLayout_UnavailableLayouts(t *testing.T) {
	// If all layouts are marked Available=false, return nil.
	unavailableLayout := &preprocess.StructLayout{TotalSize: 10, Available: false}
	vt := preprocess.VersionTable{
		"PACKET_TEST": []preprocess.VersionedLayout{
			{MinVer: 20190227, MaxVer: 0, Layout: unavailableLayout},
		},
	}
	got := resolveLayout("PACKET_TEST", 0, vt)
	if got != nil {
		t.Errorf("all-unavailable should return nil, got %v", got)
	}
}
