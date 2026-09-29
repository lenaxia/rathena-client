// Fuzz targets for the packed binary position codecs.
//
// Invariant under test: decode is a left inverse of encode for all in-range
// inputs — DecodePosDir(EncodePosDir(x, y, dir)) == (x, y, dir) once inputs
// are masked to their wire ranges (10-bit coordinates, 4-bit direction), and
// the same for the 6-byte movement format.
package packing

import "testing"

func FuzzPosDirRoundTrip(f *testing.F) {
	f.Add(uint16(0), uint16(0), uint8(0))
	f.Add(uint16(1023), uint16(1023), uint8(15))
	f.Fuzz(func(t *testing.T, x, y uint16, dir uint8) {
		mx, my := x&0x03ff, y&0x03ff
		md := dir & 0x0f
		gx, gy, gdir := func() (uint16, uint16, uint8) {
			p := EncodePosDir(mx, my, md)
			return DecodePosDir(p[:])
		}()
		if gx != mx || gy != my || gdir != md {
			t.Fatalf("round trip failed: got (%d,%d,%d), want (%d,%d,%d)", gx, gy, gdir, mx, my, md)
		}
	})
}

func FuzzMoveDataRoundTrip(f *testing.F) {
	f.Add(uint16(0), uint16(0), uint16(0), uint16(0), uint8(0), uint8(0))
	f.Add(uint16(1023), uint16(1023), uint16(1023), uint16(1023), uint8(15), uint8(15))
	f.Fuzz(func(t *testing.T, fromX, fromY, toX, toY uint16, sx0, sy0 uint8) {
		fx, fy, tx, ty, msx, msy := fromX&0x03ff, fromY&0x03ff, toX&0x03ff, toY&0x03ff, sx0&0x0f, sy0&0x0f
		gfx, gfy, gtx, gty, gsx, gsy := func() (uint16, uint16, uint16, uint16, uint8, uint8) {
			p := EncodeMoveData(fx, fy, tx, ty, msx, msy)
			return DecodeMoveData(p[:])
		}()
		if gfx != fx || gfy != fy || gtx != tx || gty != ty || gsx != msx || gsy != msy {
			t.Fatalf("round trip failed: got (%d,%d,%d,%d,%d,%d), want (%d,%d,%d,%d,%d,%d)",
				gfx, gfy, gtx, gty, gsx, gsy, fx, fy, tx, ty, msx, msy)
		}
	})
}
