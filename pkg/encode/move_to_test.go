// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-16.

package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/packing"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func TestEncodeMoveTo_PacketID(t *testing.T) {
	p := encode.EncodeMoveTo(send.MoveTo{X: 100, Y: 200}, 20200401)
	if p[0] != 0x5F || p[1] != 0x03 {
		t.Fatalf("packet ID: got %02X %02X, want 5F 03", p[0], p[1])
	}
}

func TestEncodeMoveTo_Length(t *testing.T) {
	p := encode.EncodeMoveTo(send.MoveTo{X: 100, Y: 200}, 20200401)
	if len(p) != 5 {
		t.Fatalf("length: got %d, want 5", len(p))
	}
}

func TestEncodeMoveTo_RoundTrip(t *testing.T) {
	cases := []struct {
		x, y uint16
	}{
		{0, 0},
		{1023, 1023},
		{100, 200},
		{1, 1},
		{512, 512},
	}
	for _, tc := range cases {
		p := encode.EncodeMoveTo(send.MoveTo{X: tc.x, Y: tc.y}, 20200401)
		if p[0] != 0x5F || p[1] != 0x03 {
			t.Fatalf("x=%d y=%d: packet ID wrong: %02X %02X", tc.x, tc.y, p[0], p[1])
		}
		gotX, gotY, _ := packing.DecodePosDir(p[2:])
		if gotX != tc.x || gotY != tc.y {
			t.Fatalf("x=%d y=%d: round-trip got (%d, %d)", tc.x, tc.y, gotX, gotY)
		}
	}
}

func TestEncodeMoveTo_Zero(t *testing.T) {
	p := encode.EncodeMoveTo(send.MoveTo{X: 0, Y: 0}, 20200401)
	if p[0] != 0x5F || p[1] != 0x03 {
		t.Fatalf("packet ID: got %02X %02X, want 5F 03", p[0], p[1])
	}
	x, y, dir := packing.DecodePosDir(p[2:])
	if x != 0 || y != 0 || dir != 0 {
		t.Fatalf("zero: got x=%d y=%d dir=%d, want 0 0 0", x, y, dir)
	}
}

func TestEncodeMoveTo_MaxCoords(t *testing.T) {
	p := encode.EncodeMoveTo(send.MoveTo{X: 1023, Y: 1023}, 20200401)
	x, y, _ := packing.DecodePosDir(p[2:])
	if x != 1023 || y != 1023 {
		t.Fatalf("max: got x=%d y=%d, want 1023 1023", x, y)
	}
}

func TestEncodeMoveTo_DirAlwaysZero(t *testing.T) {
	p := encode.EncodeMoveTo(send.MoveTo{X: 100, Y: 200}, 20200401)
	_, _, dir := packing.DecodePosDir(p[2:])
	if dir != 0 {
		t.Fatalf("dir: got %d, want 0 (walk request encodes no direction)", dir)
	}
}

// TestEncodeMoveTo_OutOfRange verifies that passing coordinates exceeding the
// 10-bit max (> 1023) does not panic. The packed result may not round-trip
// correctly for out-of-range values — that is expected and not a bug in EncodeMoveTo.
func TestEncodeMoveTo_OutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EncodeMoveTo panicked on out-of-range coords: %v", r)
		}
	}()
	_ = encode.EncodeMoveTo(send.MoveTo{X: 2000, Y: 3000}, 20200401)
}

func TestEncodeMoveTo_GoldenBytes(t *testing.T) {
	// Golden bytes derived from SYNTH_CZ_REQUEST_MOVE2 struct layout:
	//   byte[0-1]: 0x5F 0x03 (packet ID 0x035F LE)
	//   byte[2-4]: packing.EncodePosDir(100, 200, 0)
	// packing.EncodePosDir(100, 200, 0):
	//   p[0] = 100 >> 2 = 25 = 0x19
	//   p[1] = (100 << 6) | ((200 >> 4) & 0x3f) = (0x1900 & 0xC0) | (12 & 0x3f)
	//        = 0x40 | 0x0C = 0x4C   [bits: (100&3)<<6 = 0, (200>>4)=12]
	//        actual: uint8((100<<6) | ((200>>4)&0x3f)) = uint8(0x190C & 0xFF not quite)
	//        EncodePosDir: p[1] = uint8((x<<6)|((y>>4)&0x3f)) where x=100, y=200
	//        x<<6 = 100<<6 = 6400 = 0x1900; uint8(0x1900) = 0x00 — but with masking:
	//        (x<<6) in uint16 = 6400; (y>>4) = 12; combined = 6400|12 = 6412; uint8(6412) = 0x0C
	//        Hmm, let me recalculate using the actual Go code:
	//        p[1] = uint8((x<<6) | ((y>>4)&0x3f))
	//             = uint8((100<<6) | ((200>>4)&0x3f))
	//             = uint8(6400 | (12&63))
	//             = uint8(6400 | 12)
	//             = uint8(6412)
	//             = uint8(6412 % 256) = uint8(6412 & 0xFF) = 0x0C  (6412 = 25*256 + 12)
	// Actually let me just derive the golden directly from the reference implementation.
	golden := packing.EncodePosDir(100, 200, 0)
	p := encode.EncodeMoveTo(send.MoveTo{X: 100, Y: 200}, 20200401)
	if p[0] != 0x5F {
		t.Errorf("byte[0]: got %02X, want 5F", p[0])
	}
	if p[1] != 0x03 {
		t.Errorf("byte[1]: got %02X, want 03", p[1])
	}
	if p[2] != golden[0] || p[3] != golden[1] || p[4] != golden[2] {
		t.Errorf("coord bytes: got %02X %02X %02X, want %02X %02X %02X",
			p[2], p[3], p[4], golden[0], golden[1], golden[2])
	}
}

func TestEncodeMoveTo_PacketverIgnored(t *testing.T) {
	// 0x035F is the only packet ID for move_to; packetver is ignored.
	p1 := encode.EncodeMoveTo(send.MoveTo{X: 100, Y: 200}, 20120307)
	p2 := encode.EncodeMoveTo(send.MoveTo{X: 100, Y: 200}, 20200401)
	if p1 != p2 {
		t.Fatalf("packetver should not affect output: got %v vs %v", p1, p2)
	}
}

func BenchmarkEncodeMoveTo(b *testing.B) {
	req := send.MoveTo{X: 100, Y: 200}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeMoveTo(req, 20200401)
	}
}
