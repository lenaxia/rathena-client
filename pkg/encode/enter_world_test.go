// Manually implemented — regression test for worklog 0072.
// EncodeEnterWorld must exist and produce [0x7D, 0x00].
// The correct action is ActionEnterWorld (not ActionCzNotifyActorinit as the
// worklog proposed) — send.EnterWorld{} and ActionEnterWorld = 141 already exist.

package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeEnterWorld_WireFormat verifies the packet is always [0x7D, 0x00]
// regardless of packetver.
// Source: clif_packetdb.hpp:32 — single entry, never overridden, not in clif_shuffle.hpp.
// rAthena: PACKET_CZ_NOTIFY_ACTORINIT = 2 bytes, handler clif_parse_LoadEndAck.
func TestEncodeEnterWorld_WireFormat(t *testing.T) {
	cases := []struct {
		name string
		pv   uint32
	}{
		{"pv=20030000", 20030000},
		{"pv=20101124", 20101124},
		{"pv=20130515", 20130515},
		{"pv=20200401", 20200401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeEnterWorld(send.EnterWorld{}, tc.pv)
			if p[0] != 0x7D || p[1] != 0x00 {
				t.Errorf("pv=%d: got [%02X %02X], want [7D 00]", tc.pv, p[0], p[1])
			}
			if len(p) != 2 {
				t.Errorf("pv=%d: len=%d, want 2", tc.pv, len(p))
			}
		})
	}
}

func BenchmarkEncodeEnterWorld(b *testing.B) {
	req := send.EnterWorld{}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeEnterWorld(req, 20200401)
	}
}
