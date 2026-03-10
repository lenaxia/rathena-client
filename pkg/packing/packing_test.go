// Package packing tests verify the two packed binary position formats used by
// rAthena's wire protocol. Golden bytes are synthesized directly from the C
// implementations in src/map/clif.cpp:173–249 (rAthena). See WBUFPOS, RBUFPOS,
// WBUFPOS2, and RBUFPOS2 in that file for the authoritative reference.
package packing_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/packing"
)

// posDirCase is a single WBUFPOS / RBUFPOS test vector.
// golden bytes are computed by hand from the C formula:
//
//	p[0] = uint8(x >> 2)
//	p[1] = uint8((x << 6) | ((y >> 4) & 0x3f))
//	p[2] = uint8((y << 4) | (dir & 0x0f))
type posDirCase struct {
	name   string
	x, y   uint16
	dir    uint8
	golden [3]byte
}

var posDirCases = []posDirCase{
	{
		name: "origin_north",
		x:    0, y: 0, dir: 0,
		golden: [3]byte{0x00, 0x00, 0x00},
	},
	{
		name: "x1_y0_dir0",
		x:    1, y: 0, dir: 0,
		// p[0]=0>>2=0x00, p[1]=(1<<6)|0=0x40, p[2]=0
		golden: [3]byte{0x00, 0x40, 0x00},
	},
	{
		name: "x0_y1_dir0",
		x:    0, y: 1, dir: 0,
		// p[0]=0, p[1]=(0)|(1>>4)=0x00, p[2]=(1<<4)|0=0x10
		golden: [3]byte{0x00, 0x00, 0x10},
	},
	{
		name: "prontera_center_north",
		// Prontera center ~155,142; dir=0 (N)
		x: 155, y: 142, dir: 0,
		// x=155=0x9B: p[0]=155>>2=38=0x26; p[1]=(155<<6)|(142>>4)=((0x9B<<6)&0xFF)|8 = 0xC0|0x08=0xC8; p[2]=(142<<4)|0=0x8E0&0xFF=0xE0
		// Let me compute: x=155: 155>>2=38=0x26; 155<<6=(0xDB60)&0xFF=0x60? No:
		// x=155=10011011b; x>>2=00100110b=0x26 ✓
		// x<<6 in 8-bit = (10011011b << 6) lower byte = (11000000b) = 0xC0
		// y=142=10001110b; y>>4=00001000b=0x08; (y>>4)&0x3f=0x08
		// p[1] = 0xC0 | 0x08 = 0xC8
		// y<<4 = (10001110b<<4) lower byte = (11100000b) = 0xE0
		// dir=0; p[2] = 0xE0 | 0 = 0xE0
		golden: [3]byte{0x26, 0xC8, 0xE0},
	},
	{
		name: "max_coords_dir7",
		// x=1023=0x3FF, y=1023=0x3FF, dir=7 (NE)
		// x>>2=255=0xFF; x<<6 lower=0xC0; y>>4=63=0x3F; (y>>4)&0x3F=0x3F
		// p[1]=0xC0|0x3F=0xFF; y<<4 lower=0xF0; dir=7=0x07
		// p[2]=0xF0|0x07=0xF7
		x: 1023, y: 1023, dir: 7,
		golden: [3]byte{0xFF, 0xFF, 0xF7},
	},
	{
		name: "x4_y4_dir4_south",
		// x=4: p[0]=1; p[1]=(4<<6)&0xFF|(4>>4)=0x00; y=4: p[2]=(4<<4)|4=0x44
		x: 4, y: 4, dir: 4,
		golden: [3]byte{0x01, 0x00, 0x44},
	},
	{
		name: "x100_y200_dir2_west",
		// x=100=0x64: x>>2=25=0x19; x<<6 lower=(0x64<<6)&0xFF=0x00; wait:
		// 100=01100100b; 100>>2=00011001b=0x19 ✓
		// 100<<6: take lower 8 bits of (100*64)=6400=0x1900, lower byte=0x00
		// y=200=11001000b; y>>4=00001100b=0x0C; (y>>4)&0x3F=0x0C
		// p[1]=0x00|0x0C=0x0C
		// y<<4: take lower 8 bits of 200*16=3200=0xC80, lower byte=0x80
		// dir=2; p[2]=0x80|0x02=0x82
		x: 100, y: 200, dir: 2,
		golden: [3]byte{0x19, 0x0C, 0x82},
	},
	{
		name: "dir_all_four_bits",
		// x=0, y=0, dir=15 — tests that upper bits of dir are masked
		x: 0, y: 0, dir: 15,
		golden: [3]byte{0x00, 0x00, 0x0F},
	},
}

func TestDecodePosDir(t *testing.T) {
	for _, tc := range posDirCases {
		t.Run(tc.name, func(t *testing.T) {
			gotX, gotY, gotDir := packing.DecodePosDir(tc.golden[:])
			if gotX != tc.x || gotY != tc.y || gotDir != tc.dir {
				t.Errorf("DecodePosDir(%#v) = (%d,%d,%d); want (%d,%d,%d)",
					tc.golden, gotX, gotY, gotDir, tc.x, tc.y, tc.dir)
			}
		})
	}
}

func TestEncodePosDir(t *testing.T) {
	for _, tc := range posDirCases {
		t.Run(tc.name, func(t *testing.T) {
			got := packing.EncodePosDir(tc.x, tc.y, tc.dir)
			if got != tc.golden {
				t.Errorf("EncodePosDir(%d,%d,%d) = %#v; want %#v",
					tc.x, tc.y, tc.dir, got, tc.golden)
			}
		})
	}
}

func TestDecodePosDir_RoundTrip(t *testing.T) {
	vectors := [][3]uint16{
		{0, 0, 0}, {1023, 1023, 7}, {512, 512, 4},
		{100, 200, 2}, {155, 142, 0}, {1, 1023, 1},
	}
	for _, v := range vectors {
		x, y, dir := uint16(v[0]), uint16(v[1]), uint8(v[2])
		encoded := packing.EncodePosDir(x, y, dir)
		gotX, gotY, gotDir := packing.DecodePosDir(encoded[:])
		if gotX != x || gotY != y || gotDir != dir {
			t.Errorf("round-trip fail: encode(%d,%d,%d)=%#v → decode=(%d,%d,%d)",
				x, y, dir, encoded, gotX, gotY, gotDir)
		}
	}
}

// moveDataCase is a single WBUFPOS2 / RBUFPOS2 test vector.
// golden bytes are computed from:
//
//	p[0] = uint8(x0 >> 2)
//	p[1] = uint8((x0 << 6) | ((y0 >> 4) & 0x3f))
//	p[2] = uint8((y0 << 4) | ((x1 >> 6) & 0x0f))
//	p[3] = uint8((x1 << 2) | ((y1 >> 8) & 0x03))
//	p[4] = uint8(y1)
//	p[5] = uint8((sx0 << 4) | (sy0 & 0x0f))
type moveDataCase struct {
	name                   string
	fromX, fromY, toX, toY uint16
	sx0, sy0               uint8
	golden                 [6]byte
}

var moveDataCases = []moveDataCase{
	{
		name:  "all_zero",
		fromX: 0, fromY: 0, toX: 0, toY: 0, sx0: 0, sy0: 0,
		golden: [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	},
	{
		name: "max_coords_max_offsets",
		// fromX=1023,fromY=1023,toX=1023,toY=1023,sx0=15,sy0=15
		// p[0]=0xFF; p[1]=0xFF; p[2]=(1023<<4)lower|(1023>>6)&0x0f
		// 1023<<4 lower=(0xF)<<4=0xF0; 1023>>6=15=0x0F; p[2]=0xFF
		// p[3]=(1023<<2)lower|(1023>>8)&0x03; 1023<<2 lower=(0xFC); 1023>>8=3; p[3]=0xFF
		// p[4]=1023&0xFF=0xFF; p[5]=(15<<4)|(15&0xF)=0xFF
		fromX: 1023, fromY: 1023, toX: 1023, toY: 1023, sx0: 15, sy0: 15,
		golden: [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	},
	{
		name: "prontera_short_move",
		// fromX=155,fromY=142 → toX=156,toY=142, sx0=8,sy0=8
		// p[0]=155>>2=0x26
		// p[1]=(155<<6)&0xFF=0xC0|((142>>4)&0x3F)=0xC0|0x08=0xC8
		// p[2]=(142<<4)&0xFF=0xE0|((156>>6)&0x0F)=0xE0|0x02=0xE2
		// 156>>6=2 ✓ (156=0x9C; 0x9C>>6=2)
		// p[3]=(156<<2)&0xFF=0x70|(142>>8&0x03)=0x70|0x00=0x70
		// 156<<2=624=0x270; lower=0x70 ✓
		// p[4]=142&0xFF=0x8E
		// p[5]=(8<<4)|(8&0x0F)=0x88
		fromX: 155, fromY: 142, toX: 156, toY: 142, sx0: 8, sy0: 8,
		golden: [6]byte{0x26, 0xC8, 0xE2, 0x70, 0x8E, 0x88},
	},
	{
		name: "sx0_sy0_independence",
		// x0=x1=y0=y1=0; sx0=5,sy0=3 → p[5]=(5<<4)|3=0x53
		fromX: 0, fromY: 0, toX: 0, toY: 0, sx0: 5, sy0: 3,
		golden: [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x53},
	},
	{
		name: "toY_crosses_byte_boundary",
		// toY=256=0x100: p[3] gets bit 8 of y1 in bits[1:0]; p[4]=0x00
		// fromX=0,fromY=0,toX=0,toY=256
		// p[0]=0;p[1]=0;p[2]=(0<<4)|((0>>6)&0x0F)=0
		// p[3]=(0<<2)|((256>>8)&0x03)=0|1=0x01
		// p[4]=256&0xFF=0x00
		fromX: 0, fromY: 0, toX: 0, toY: 256, sx0: 0, sy0: 0,
		golden: [6]byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00},
	},
}

func TestDecodeMoveData(t *testing.T) {
	for _, tc := range moveDataCases {
		t.Run(tc.name, func(t *testing.T) {
			fromX, fromY, toX, toY, sx0, sy0 := packing.DecodeMoveData(tc.golden[:])
			if fromX != tc.fromX || fromY != tc.fromY ||
				toX != tc.toX || toY != tc.toY ||
				sx0 != tc.sx0 || sy0 != tc.sy0 {
				t.Errorf("DecodeMoveData(%#v) = (%d,%d,%d,%d,%d,%d); want (%d,%d,%d,%d,%d,%d)",
					tc.golden, fromX, fromY, toX, toY, sx0, sy0,
					tc.fromX, tc.fromY, tc.toX, tc.toY, tc.sx0, tc.sy0)
			}
		})
	}
}

func TestEncodeMoveData(t *testing.T) {
	for _, tc := range moveDataCases {
		t.Run(tc.name, func(t *testing.T) {
			got := packing.EncodeMoveData(tc.fromX, tc.fromY, tc.toX, tc.toY, tc.sx0, tc.sy0)
			if got != tc.golden {
				t.Errorf("EncodeMoveData(%d,%d,%d,%d,%d,%d) = %#v; want %#v",
					tc.fromX, tc.fromY, tc.toX, tc.toY, tc.sx0, tc.sy0, got, tc.golden)
			}
		})
	}
}

func TestDecodeMoveData_RoundTrip(t *testing.T) {
	vectors := [][6]uint16{
		{0, 0, 0, 0, 0, 0},
		{1023, 1023, 1023, 1023, 15, 15},
		{100, 200, 101, 199, 8, 8},
		{155, 142, 156, 142, 0, 0},
		{512, 512, 513, 511, 5, 3},
		{0, 0, 0, 256, 0, 0},
	}
	for _, v := range vectors {
		fromX, fromY, toX, toY := uint16(v[0]), uint16(v[1]), uint16(v[2]), uint16(v[3])
		sx0, sy0 := uint8(v[4]), uint8(v[5])
		encoded := packing.EncodeMoveData(fromX, fromY, toX, toY, sx0, sy0)
		gFromX, gFromY, gToX, gToY, gSx0, gSy0 := packing.DecodeMoveData(encoded[:])
		if gFromX != fromX || gFromY != fromY || gToX != toX || gToY != toY ||
			gSx0 != sx0 || gSy0 != sy0 {
			t.Errorf("round-trip fail: encode(%d,%d,%d,%d,%d,%d)=%#v → decode=(%d,%d,%d,%d,%d,%d)",
				fromX, fromY, toX, toY, sx0, sy0, encoded,
				gFromX, gFromY, gToX, gToY, gSx0, gSy0)
		}
	}
}

// TestDecodeMoveData_ByteFiveIsNotDirection documents that byte 5 of the
// 6-byte format is NOT a direction — it is sx0 (high nibble) and sy0 (low nibble).
// This is a regression test against the known goKore v1 bug
// (handlers/actors/handler.go:88: direction = (data[5] & 0xF0) >> 4).
func TestDecodeMoveData_ByteFiveIsNotDirection(t *testing.T) {
	// Encode a known move with sx0=0xA, sy0=0x5 → byte 5 should be 0xA5
	encoded := packing.EncodeMoveData(100, 200, 101, 199, 0xA, 0x5)
	if encoded[5] != 0xA5 {
		t.Fatalf("byte 5 should be 0xA5 (sx0=0xA, sy0=0x5); got 0x%02X", encoded[5])
	}
	// Decode — sx0 and sy0 should come back correctly, not be interpreted as direction
	_, _, _, _, sx0, sy0 := packing.DecodeMoveData(encoded[:])
	if sx0 != 0xA || sy0 != 0x5 {
		t.Errorf("byte5=0xA5 should decode to sx0=10,sy0=5; got sx0=%d,sy0=%d", sx0, sy0)
	}
}

func FuzzDecodePosDir(f *testing.F) {
	for _, tc := range posDirCases {
		f.Add(tc.golden[0], tc.golden[1], tc.golden[2])
	}
	f.Fuzz(func(t *testing.T, b0, b1, b2 byte) {
		data := []byte{b0, b1, b2}
		x, y, dir := packing.DecodePosDir(data)
		if x > 1023 || y > 1023 {
			t.Errorf("DecodePosDir produced out-of-range coordinate: x=%d y=%d", x, y)
		}
		if dir > 15 {
			t.Errorf("DecodePosDir produced out-of-range dir: %d", dir)
		}
		re := packing.EncodePosDir(x, y, dir)
		rx, ry, rdir := packing.DecodePosDir(re[:])
		if rx != x || ry != y || rdir != dir {
			t.Errorf("encode(decode(%#v)) round-trip failed: got (%d,%d,%d) then (%d,%d,%d)",
				data, x, y, dir, rx, ry, rdir)
		}
	})
}

func FuzzDecodeMoveData(f *testing.F) {
	for _, tc := range moveDataCases {
		f.Add(tc.golden[0], tc.golden[1], tc.golden[2], tc.golden[3], tc.golden[4], tc.golden[5])
	}
	f.Fuzz(func(t *testing.T, b0, b1, b2, b3, b4, b5 byte) {
		data := []byte{b0, b1, b2, b3, b4, b5}
		fromX, fromY, toX, toY, sx0, sy0 := packing.DecodeMoveData(data)
		if fromX > 1023 || fromY > 1023 || toX > 1023 || toY > 1023 {
			t.Errorf("DecodeMoveData produced out-of-range coordinates")
		}
		if sx0 > 15 || sy0 > 15 {
			t.Errorf("DecodeMoveData produced out-of-range sub-offsets: sx0=%d sy0=%d", sx0, sy0)
		}
		re := packing.EncodeMoveData(fromX, fromY, toX, toY, sx0, sy0)
		rfX, rfY, rtX, rtY, rsx0, rsy0 := packing.DecodeMoveData(re[:])
		if rfX != fromX || rfY != fromY || rtX != toX || rtY != toY ||
			rsx0 != sx0 || rsy0 != sy0 {
			t.Errorf("encode(decode(%#v)) round-trip failed", data)
		}
	})
}

func BenchmarkDecodePosDir(b *testing.B) {
	data := packing.EncodePosDir(155, 142, 0)
	b.ReportAllocs()
	for b.Loop() {
		packing.DecodePosDir(data[:])
	}
}

func BenchmarkEncodePosDir(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		packing.EncodePosDir(155, 142, 0)
	}
}

func BenchmarkDecodeMoveData(b *testing.B) {
	data := packing.EncodeMoveData(155, 142, 156, 142, 8, 8)
	b.ReportAllocs()
	for b.Loop() {
		packing.DecodeMoveData(data[:])
	}
}

func BenchmarkEncodeMoveData(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		packing.EncodeMoveData(155, 142, 156, 142, 8, 8)
	}
}

// FuzzEncodeMoveData_Sx0OutOfRange verifies that sx0 values outside 0-15 are
// correctly masked before encoding, and that the round-trip is stable.
// This is a regression fuzz test for Bug 14-D (packing.go:88 missing sx0 mask).
func FuzzEncodeMoveData_Sx0OutOfRange(f *testing.F) {
	// seed with out-of-range sx0 values to exercise the bug fix
	f.Add(uint16(100), uint16(200), uint16(101), uint16(201), uint8(0x10), uint8(8))
	f.Add(uint16(100), uint16(200), uint16(101), uint16(201), uint8(0xFF), uint8(0xFF))
	f.Add(uint16(100), uint16(200), uint16(101), uint16(201), uint8(16), uint8(16))
	f.Add(uint16(512), uint16(512), uint16(513), uint16(512), uint8(0x1F), uint8(0x0F))
	f.Fuzz(func(t *testing.T, fromX, toX uint16, fromY, toY uint16, sx0, sy0 uint8) {
		// clamp coords to valid 10-bit range
		fromX &= 0x3FF
		fromY &= 0x3FF
		toX &= 0x3FF
		toY &= 0x3FF
		encoded := packing.EncodeMoveData(fromX, fromY, toX, toY, sx0, sy0)
		// decode and verify sx0/sy0 round-trip with masking applied
		_, _, _, _, decSx0, decSy0 := packing.DecodeMoveData(encoded[:])
		if decSx0 != sx0&0x0f {
			t.Errorf("sx0 round-trip: encoded sx0=%d, decoded sx0=%d (want %d)",
				sx0, decSx0, sx0&0x0f)
		}
		if decSy0 != sy0&0x0f {
			t.Errorf("sy0 round-trip: encoded sy0=%d, decoded sy0=%d (want %d)",
				sy0, decSy0, sy0&0x0f)
		}
	})
}
