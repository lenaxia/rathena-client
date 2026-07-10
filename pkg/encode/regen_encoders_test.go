// Tests for encoders whose wire format changed in worklog 0091's
// resolveLayout fix. Verifies exact wire-byte layout against rAthena
// packets_struct.hpp, since these are the two encoders that had latent
// codegen bugs before the resolveLayout newest-fallback landed.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeCzReqGuildEmblemImg2_WireFormat verifies the encoder produces
// the 14-byte layout for the pv >= 20190619 variant, which is what modern
// rAthena (including goKore's target pv=20200401) expects on the wire.
//
// Source: rAthena src/map/packets_struct.hpp:5788-5798
//
//	struct PACKET_CZ_REQ_GUILD_EMBLEM_IMG2 {
//	    int16 packetType;   // offset 0, 2 bytes = 0x0B1E
//	    int32 guild_id;     // offset 2, 4 bytes
//	    int32 emblem_id;    // offset 6, 4 bytes
//	    int32 unused;       // offset 10, 4 bytes (added at pv >= 20190619)
//	}  // total = 14 bytes
func TestEncodeCzReqGuildEmblemImg2_WireFormat(t *testing.T) {
	req := send.CzReqGuildEmblemImg2{
		Guild_id:  0x11223344,
		Emblem_id: 0x55667788,
		Unused:    -0x66554434, // 0x99AABBCC as signed int32
	}
	got := encode.EncodeCzReqGuildEmblemImg2(req, 20200401)

	if len(got) != 14 {
		t.Fatalf("wire size: got %d bytes, want 14 (rAthena packets_struct.hpp:5788-5798)", len(got))
	}
	if id := binary.LittleEndian.Uint16(got[0:2]); id != 0x0B1E {
		t.Errorf("packet id: got 0x%04X, want 0x0B1E", id)
	}
	if v := binary.LittleEndian.Uint32(got[2:6]); v != 0x11223344 {
		t.Errorf("guild_id: got 0x%08X, want 0x11223344", v)
	}
	if v := binary.LittleEndian.Uint32(got[6:10]); v != 0x55667788 {
		t.Errorf("emblem_id: got 0x%08X, want 0x55667788", v)
	}
	if v := binary.LittleEndian.Uint32(got[10:14]); v != 0x99AABBCC {
		t.Errorf("unused: got 0x%08X, want 0x99AABBCC", v)
	}
}

// TestEncodeCzReqTakeoffEquipAll_WireFormat exercises both branches of the
// pv-branching encoder. rAthena binds ZC_REQ_TAKEOFF_EQUIP_ALL to two
// DIFFERENT packet IDs across pv=20230906 with different struct sizes.
//
// Source: rAthena src/map/packets_struct.hpp:5166-5177
//
//	pv >= 20230906 → PACKET_CZ_REQ_TAKEOFF_EQUIP_ALL 6 bytes, id 0x0BF5,
//	                 with uint32 location at offset 2
//	pv >= 20210818 → PACKET_CZ_REQ_TAKEOFF_EQUIP_ALL 2 bytes, id 0x0BAD
//
// At packetvers below 20210818 the packet doesn't exist; the encoder
// panics rather than emitting a wrong wire format.
func TestEncodeCzReqTakeoffEquipAll_WireFormat_v20210818(t *testing.T) {
	req := send.CzReqTakeoffEquipAll{Location: 0x11223344}
	got := encode.EncodeCzReqTakeoffEquipAll(req, 20210818)
	if len(got) != 2 {
		t.Fatalf("wire size at pv=20210818: got %d bytes, want 2", len(got))
	}
	if id := binary.LittleEndian.Uint16(got[0:2]); id != 0x0BAD {
		t.Errorf("packet id at pv=20210818: got 0x%04X, want 0x0BAD", id)
	}
}

func TestEncodeCzReqTakeoffEquipAll_WireFormat_v20230906(t *testing.T) {
	req := send.CzReqTakeoffEquipAll{Location: 0x11223344}
	got := encode.EncodeCzReqTakeoffEquipAll(req, 20230906)
	if len(got) != 6 {
		t.Fatalf("wire size at pv=20230906: got %d bytes, want 6", len(got))
	}
	if id := binary.LittleEndian.Uint16(got[0:2]); id != 0x0BF5 {
		t.Errorf("packet id at pv=20230906: got 0x%04X, want 0x0BF5", id)
	}
	if v := binary.LittleEndian.Uint32(got[2:6]); v != 0x11223344 {
		t.Errorf("location: got 0x%08X, want 0x11223344", v)
	}
}

func TestEncodeCzReqTakeoffEquipAll_PanicsBelowRange(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic at pv=20200401 (packet doesn't exist), got nothing")
		}
	}()
	encode.EncodeCzReqTakeoffEquipAll(send.CzReqTakeoffEquipAll{}, 20200401)
}

// Benchmarks for the two changed encoders. Convention: run at goKore's
// target pv=20200401 for CzReqGuildEmblemImg2 (valid at that pv), and at
// each of the two branches for CzReqTakeoffEquipAll.
//
// Rule 4 target: 0 allocs/op. Verified locally: all three encoders
// including the []byte-returning takeoff variant hit 0 allocs/op — Go's
// escape analysis proves the make([]byte, N) doesn't escape for these
// tiny-payload single-return use cases.

func BenchmarkEncodeCzReqGuildEmblemImg2(b *testing.B) {
	req := send.CzReqGuildEmblemImg2{
		Guild_id:  0x11223344,
		Emblem_id: 0x55667788,
		Unused:    0,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeCzReqGuildEmblemImg2(req, 20200401)
	}
}

func BenchmarkEncodeCzReqTakeoffEquipAll_v20210818(b *testing.B) {
	req := send.CzReqTakeoffEquipAll{Location: 0x11223344}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeCzReqTakeoffEquipAll(req, 20210818)
	}
}

func BenchmarkEncodeCzReqTakeoffEquipAll_v20230906(b *testing.B) {
	req := send.CzReqTakeoffEquipAll{Location: 0x11223344}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeCzReqTakeoffEquipAll(req, 20230906)
	}
}
