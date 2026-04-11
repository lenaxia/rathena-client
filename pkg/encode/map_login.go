// Manually implemented — codegen cannot express the non-contiguous 23-byte
// windows for 0x0436 (CZ_ENTER2). See docs/WORKLOG/0082_*.md.

package encode

import (
	"github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeMapLogin encodes a 0x0436 (CZ_ENTER / CZ_ENTER2) packet.
//
// The wire length depends on packetver:
//
//   - 19 bytes (sex at offset 18) in all other cases:
//     id(2) + AID(4) + GID(4) + AuthCode(4) + clientTime(4) + sex(1)
//     Source: clif_shuffle.hpp:4747
//     parseable_packet( 0x0436, 19, clif_parse_WantToConnection, 2, 6, 10, 14, 18 )
//
//   - 23 bytes (sex at offset 22) when:
//     PACKETVER_RE_NUM >= 20211103  → packetver in [20211103, 20211118]
//     PACKETVER_MAIN_NUM >= 20220330 → packetver >= 20220330 (outside RE window)
//     id(2) + AID(4) + GID(4) + AuthCode(4) + clientTime(4) + tick(4) + sex(1)
//     Source: clif_shuffle.hpp:4744-4745
//     #if PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330
//     parseable_packet( 0x0436, 23, clif_parse_WantToConnection, 2, 6, 10, 14, 22 )
//
// rAthena config/packets.hpp line 22: PACKETVER_RE is defined (→ PACKETVER_RE_NUM=PACKETVER) when
// (PACKETVER > 20151104 && PACKETVER < 20180704) || (PACKETVER >= 20200902 && PACKETVER <= 20211118).
// GCC-verified boundaries: 20211103→23B, 20211118→23B, 20211119→19B, 20220329→19B, 20220330→23B.
func EncodeMapLogin(req send.MapLogin, packetver uint32) []byte {
	// 23-byte variant: PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330
	// Source: clif_shuffle.hpp:4744-4745
	if (packetver >= 20211103 && packetver <= 20211118) || packetver >= 20220330 {
		p := make([]byte, 23)
		p[0] = 0x36
		p[1] = 0x04
		leU32Put(p[2:], req.AID)               // rAthena: AID        (pos[0]=2)
		leU32Put(p[6:], req.GID)               // rAthena: GID        (pos[1]=6)
		leU32Put(p[10:], uint32(req.AuthCode)) // rAthena: AuthCode   (pos[2]=10)
		leU32Put(p[14:], req.ClientTime)       // rAthena: clientTick (pos[3]=14)
		// p[18:22] tick = 0                   // extra field
		p[22] = req.Sex // rAthena: sex        (pos[4]=22)
		return p
	}
	// 19-byte variant: default
	// Source: clif_shuffle.hpp:4747
	p := make([]byte, 19)
	p[0] = 0x36
	p[1] = 0x04
	leU32Put(p[2:], req.AID)               // rAthena: AID        (pos[0]=2)
	leU32Put(p[6:], req.GID)               // rAthena: GID        (pos[1]=6)
	leU32Put(p[10:], uint32(req.AuthCode)) // rAthena: AuthCode   (pos[2]=10)
	leU32Put(p[14:], req.ClientTime)       // rAthena: clientTick (pos[3]=14)
	p[18] = req.Sex                        // rAthena: sex        (pos[4]=18)
	return p
}
