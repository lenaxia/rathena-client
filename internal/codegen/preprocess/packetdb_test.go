package preprocess_test

import (
	"testing"

	"github.com/lenaxia/ragnarok-go-client/internal/codegen/preprocess"
)

func TestParsePacketDB_Basic(t *testing.T) {
	content := `
	packet(0x0064,55);
	packet(0x0065,17);
	parseable_packet(0x0085,5,clif_parse_WalkToXY,2);
	parseable_packet(0x007e,6,clif_parse_TickSend,2);
	packet(0x0069,-1);
`
	entries, err := preprocess.ParsePacketDB(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("entry count = %d, want 5", len(entries))
	}

	// Check 0x0064
	e := entries[0]
	if e.ID != 0x0064 || e.Length != 55 || e.Handler != "" {
		t.Errorf("entry[0] = {%04x, %d, %q}, want {0x0064, 55, \"\"}", e.ID, e.Length, e.Handler)
	}
	// Check 0x0085
	e = entries[2]
	if e.ID != 0x0085 || e.Length != 5 || e.Handler != "clif_parse_WalkToXY" {
		t.Errorf("entry[2] = {%04x, %d, %q}, want {0x0085, 5, \"clif_parse_WalkToXY\"}", e.ID, e.Length, e.Handler)
	}
	// Check variable length
	e = entries[4]
	if e.ID != 0x0069 || e.Length != -1 {
		t.Errorf("entry[4] = {%04x, %d}, want {0x0069, -1}", e.ID, e.Length)
	}
}

func TestParseShuffle_TwoSections(t *testing.T) {
	content := `
#if PACKETVER == 20130515
	parseable_packet(0x0437,5,clif_parse_WalkToXY,2);
	parseable_packet(0x0369,7,clif_parse_ActionRequest,2,6);
	parseable_packet(0x035F,6,clif_parse_TickSend,2);
#elif PACKETVER == 20130522
	parseable_packet(0x0360,5,clif_parse_WalkToXY,2);
	parseable_packet(0x07EC,6,clif_parse_TickSend,2);
#endif
`
	sections, err := preprocess.ParseShuffle(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("section count = %d, want 2", len(sections))
	}

	s0 := sections[0]
	if s0.PacketVer != 20130515 {
		t.Errorf("sections[0].PacketVer = %d, want 20130515", s0.PacketVer)
	}
	if len(s0.Entries) != 3 {
		t.Errorf("sections[0] entry count = %d, want 3", len(s0.Entries))
	}

	// Walk packet in 20130515 should be 0x0437
	found := false
	for _, e := range s0.Entries {
		if e.Handler == "clif_parse_WalkToXY" {
			if e.ID != 0x0437 {
				t.Errorf("20130515 WalkToXY ID = 0x%04X, want 0x0437", e.ID)
			}
			found = true
		}
	}
	if !found {
		t.Error("clif_parse_WalkToXY not found in 20130515 section")
	}

	s1 := sections[1]
	if s1.PacketVer != 20130522 {
		t.Errorf("sections[1].PacketVer = %d, want 20130522", s1.PacketVer)
	}
	// Walk packet in 20130522 should be 0x0360
	for _, e := range s1.Entries {
		if e.Handler == "clif_parse_WalkToXY" {
			if e.ID != 0x0360 {
				t.Errorf("20130522 WalkToXY ID = 0x%04X, want 0x0360", e.ID)
			}
		}
	}
}

func TestParseObfuscationKeys_Basic(t *testing.T) {
	// Actual preprocessed output format from clif_obfuscation.hpp via g++ -E -P.
	// The packet_keys(a,b,c) macro expands to:
	//   static uint32 clif_cryptKey[] = { a, b, c };
	preprocessed := `  static uint32 clif_cryptKey[] = { 0x13F2F5D1, 0x1B9B4FFF, 0x0ADF2B7B };;
`
	k0, k1, k2 := preprocess.ParseObfuscationKeys(preprocessed)
	if k0 != 0x13F2F5D1 {
		t.Errorf("k0 = 0x%08X, want 0x13F2F5D1", k0)
	}
	if k1 != 0x1B9B4FFF {
		t.Errorf("k1 = 0x%08X, want 0x1B9B4FFF", k1)
	}
	if k2 != 0x0ADF2B7B {
		t.Errorf("k2 = 0x%08X, want 0x0ADF2B7B", k2)
	}
}

func TestParseObfuscationKeys_NoKeys(t *testing.T) {
	// Empty / no keys = obfuscation disabled.
	k0, k1, k2 := preprocess.ParseObfuscationKeys("")
	if k0 != 0 || k1 != 0 || k2 != 0 {
		t.Errorf("empty: got (%x,%x,%x), want (0,0,0)", k0, k1, k2)
	}
}
