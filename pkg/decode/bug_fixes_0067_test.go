// Tests for bug fixes in 0067:
//   BUG-2:     AreaSpell 0x08C7 fix + 0x099F/0x09CA new decoders
//   BUG-NEW-1: ItemAppeared 0x084B / 0x0ADD decoders
//   BUG-NEW-2: ActorStatusActive 0x0983 decoder
//   BUG-NEW-3: Actor middle-gen decoders (0x07F9/0x0857/0x0915 exists,
//              0x07F8/0x0858/0x090F connected, 0x07F7/0x0856/0x0914 moved)
//
// Each golden test constructs bytes directly from the rAthena struct layout
// (GCC-verified) and asserts field values — no logic without evidence.

package decode

import (
	"encoding/binary"
	"testing"
)

// ── AreaSpell ─────────────────────────────────────────────────────────────────

// TestAreaSpell_0x08C7_Fix verifies the OOB-read bug is fixed.
// pv=20110718: 19-byte packet. Old decoder read data[19] (out of bounds).
// packet_skill_entry layout: PacketType(2)+PacketLength(2)+AID(4)+creatorAID(4)+xPos(2)+yPos(2)+job(1)+RadiusRange(1)+isVisible(1)
func TestAreaSpell_0x08C7_Fix(t *testing.T) {
	pv := uint32(20110718)
	b := make([]byte, 19)
	binary.LittleEndian.PutUint16(b[0:], 0x08C7)  // PacketType
	binary.LittleEndian.PutUint16(b[2:], 19)      // PacketLength
	binary.LittleEndian.PutUint32(b[4:], 0xABCDE) // AID
	binary.LittleEndian.PutUint32(b[8:], 0x11111) // creatorAID
	binary.LittleEndian.PutUint16(b[12:], 100)    // xPos
	binary.LittleEndian.PutUint16(b[14:], 200)    // yPos
	b[16] = 5                                     // job (uint8)
	b[17] = 3                                     // RadiusRange (int8)
	b[18] = 1                                     // isVisible

	e := AreaSpell_0x08C7(b, pv)

	if e.AID != 0xABCDE {
		t.Errorf("AID: got %#x, want 0xABCDE", e.AID)
	}
	if e.CreatorAID != 0x11111 {
		t.Errorf("CreatorAID: got %#x, want 0x11111", e.CreatorAID)
	}
	if e.XPos != 100 {
		t.Errorf("XPos: got %d, want 100", e.XPos)
	}
	if e.YPos != 200 {
		t.Errorf("YPos: got %d, want 200", e.YPos)
	}
	if e.Job != 5 {
		t.Errorf("Job: got %d, want 5", e.Job)
	}
	if e.RadiusRange != 3 {
		t.Errorf("RadiusRange: got %d, want 3", e.RadiusRange)
	}
	if e.IsVisible != 1 {
		t.Errorf("IsVisible: got %d, want 1", e.IsVisible)
	}
}

// TestAreaSpell_0x099F_Golden verifies the new 0x099F decoder (22-byte, int32 job).
func TestAreaSpell_0x099F_Golden(t *testing.T) {
	b := make([]byte, 22)
	binary.LittleEndian.PutUint16(b[0:], 0x099F)
	binary.LittleEndian.PutUint16(b[2:], 22)
	binary.LittleEndian.PutUint32(b[4:], 0xFEDC) // AID
	binary.LittleEndian.PutUint32(b[8:], 0x1234) // creatorAID
	binary.LittleEndian.PutUint16(b[12:], 55)    // xPos
	binary.LittleEndian.PutUint16(b[14:], 77)    // yPos
	binary.LittleEndian.PutUint32(b[16:], 7)     // job (int32)
	b[20] = 4                                    // RadiusRange
	b[21] = 1                                    // isVisible

	e := AreaSpell_0x099F(b, 20121212)

	if e.AID != 0xFEDC {
		t.Errorf("AID: got %#x, want 0xFEDC", e.AID)
	}
	if e.Job != 7 {
		t.Errorf("Job: got %d, want 7", e.Job)
	}
	if e.RadiusRange != 4 {
		t.Errorf("RadiusRange: got %d, want 4", e.RadiusRange)
	}
	if e.IsVisible != 1 {
		t.Errorf("IsVisible: got %d, want 1", e.IsVisible)
	}
}

// TestAreaSpell_0x09CA_Golden verifies the new 0x09CA decoder (23-byte, +level).
func TestAreaSpell_0x09CA_Golden(t *testing.T) {
	b := make([]byte, 23)
	binary.LittleEndian.PutUint16(b[0:], 0x09CA)
	binary.LittleEndian.PutUint16(b[2:], 23)
	binary.LittleEndian.PutUint32(b[4:], 0x9999) // AID
	binary.LittleEndian.PutUint32(b[8:], 0x8888) // creatorAID
	binary.LittleEndian.PutUint16(b[12:], 10)    // xPos
	binary.LittleEndian.PutUint16(b[14:], 20)    // yPos
	binary.LittleEndian.PutUint32(b[16:], 83)    // job
	b[20] = 2                                    // RadiusRange
	b[21] = 1                                    // isVisible
	b[22] = 5                                    // level

	e := AreaSpell_0x09CA(b, 20130731)

	if e.Job != 83 {
		t.Errorf("Job: got %d, want 83", e.Job)
	}
	if e.Level != 5 {
		t.Errorf("Level: got %d, want 5", e.Level)
	}
}

// ── ItemAppeared ─────────────────────────────────────────────────────────────

// TestItemAppeared_0x009E_Unchanged verifies the refactored 0x009E decoder still works.
// Old decoder layout: ITAID(4)+ITID(2)+IsIdentified(1)+xPos(2)+yPos(2)+subX(1)+subY(1)+count(2) = 17 bytes.
func TestItemAppeared_0x009E_Golden(t *testing.T) {
	b := make([]byte, 17)
	binary.LittleEndian.PutUint16(b[0:], 0x009E)
	binary.LittleEndian.PutUint32(b[2:], 0xAABB) // ITAID
	binary.LittleEndian.PutUint16(b[6:], 501)    // ITID (uint16)
	b[8] = 1                                     // IsIdentified
	binary.LittleEndian.PutUint16(b[9:], 150)    // xPos
	binary.LittleEndian.PutUint16(b[11:], 250)   // yPos
	b[13] = 2                                    // subX
	b[14] = 3                                    // subY
	binary.LittleEndian.PutUint16(b[15:], 99)    // count

	e := ItemAppeared_0x009E(b, 20060101)

	if e.ITAID != 0xAABB {
		t.Errorf("ITAID: got %#x, want 0xAABB", e.ITAID)
	}
	if e.ITID != 501 {
		t.Errorf("ITID: got %d, want 501", e.ITID)
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", e.IsIdentified)
	}
	if e.Count != 99 {
		t.Errorf("Count: got %d, want 99", e.Count)
	}
}

// TestItemAppeared_0x084B_Golden: 19-byte packet, adds Type field.
// Layout: PacketType(2)+ITAID(4)+ITID(2)+type(2)+IsIdentified(1)+xPos(2)+yPos(2)+subX(1)+subY(1)+count(2)
func TestItemAppeared_0x084B_Golden(t *testing.T) {
	b := make([]byte, 19)
	binary.LittleEndian.PutUint16(b[0:], 0x084B)
	binary.LittleEndian.PutUint32(b[2:], 0xCCDD) // ITAID
	binary.LittleEndian.PutUint16(b[6:], 999)    // ITID (uint16)
	binary.LittleEndian.PutUint16(b[8:], 4)      // type
	b[10] = 1                                    // IsIdentified
	binary.LittleEndian.PutUint16(b[11:], 100)   // xPos
	binary.LittleEndian.PutUint16(b[13:], 200)   // yPos
	b[15] = 1                                    // subX
	b[16] = 2                                    // subY
	binary.LittleEndian.PutUint16(b[17:], 1)     // count

	e := ItemAppeared_0x084B(b, 20130320)

	if e.ITAID != 0xCCDD {
		t.Errorf("ITAID: got %#x, want 0xCCDD", e.ITAID)
	}
	if e.ITID != 999 {
		t.Errorf("ITID: got %d, want 999", e.ITID)
	}
	if e.Type != 4 {
		t.Errorf("Type: got %d, want 4", e.Type)
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", e.IsIdentified)
	}
}

// TestItemAppeared_0x0ADD_Modern: 24-byte packet at pv=20200401 (ITID uint32).
// Layout: PacketType(2)+ITAID(4)+ITID(4)+type(2)+IsIdentified(1)+xPos(2)+yPos(2)+subX(1)+subY(1)+count(2)+showdropeffect(1)+dropeffectmode(2)
func TestItemAppeared_0x0ADD_Modern(t *testing.T) {
	b := make([]byte, 24)
	binary.LittleEndian.PutUint16(b[0:], 0x0ADD)
	binary.LittleEndian.PutUint32(b[2:], 0xEEFF)     // ITAID
	binary.LittleEndian.PutUint32(b[6:], 0x0001869F) // ITID (uint32 at pv>=20181121)
	binary.LittleEndian.PutUint16(b[10:], 6)         // type
	b[12] = 1                                        // IsIdentified
	binary.LittleEndian.PutUint16(b[13:], 55)        // xPos
	binary.LittleEndian.PutUint16(b[15:], 66)        // yPos
	b[17] = 0                                        // subX
	b[18] = 0                                        // subY
	binary.LittleEndian.PutUint16(b[19:], 1)         // count
	b[21] = 1                                        // showdropeffect
	binary.LittleEndian.PutUint16(b[22:], 0x0003)    // dropeffectmode

	e := ItemAppeared_0x0ADD(b, 20200401)

	if e.ITID != 0x0001869F {
		t.Errorf("ITID: got %#x, want 0x0001869F", e.ITID)
	}
	if e.Type != 6 {
		t.Errorf("Type: got %d, want 6", e.Type)
	}
	if e.Showdropeffect != 1 {
		t.Errorf("Showdropeffect: got %d, want 1", e.Showdropeffect)
	}
	if e.Dropeffectmode != 3 {
		t.Errorf("Dropeffectmode: got %d, want 3", e.Dropeffectmode)
	}
}

// ── ActorStatusActive ─────────────────────────────────────────────────────────

// TestActorStatusActive_0x0983_Golden: 29-byte packet with Total+Left+val1-3.
// Layout: PacketType(2)+index(2)+AID(4)+state(1)+Total(4)+Left(4)+val1(4)+val2(4)+val3(4)
func TestActorStatusActive_0x0983_Golden(t *testing.T) {
	b := make([]byte, 29)
	binary.LittleEndian.PutUint16(b[0:], 0x0983)
	binary.LittleEndian.PutUint16(b[2:], 42)     // index
	binary.LittleEndian.PutUint32(b[4:], 0xDEAD) // AID
	b[8] = 7                                     // state (SC_BLESSING)
	binary.LittleEndian.PutUint32(b[9:], 60000)  // Total (60s in ms)
	binary.LittleEndian.PutUint32(b[13:], 30000) // Left (30s remaining)
	binary.LittleEndian.PutUint32(b[17:], 10)    // val1
	binary.LittleEndian.PutUint32(b[21:], 0)     // val2
	binary.LittleEndian.PutUint32(b[25:], 0)     // val3

	e := ActorStatusActive_0x0983(b, 20200401)

	if e.Index != 42 {
		t.Errorf("Index: got %d, want 42", e.Index)
	}
	if e.AID != 0xDEAD {
		t.Errorf("AID: got %#x, want 0xDEAD", e.AID)
	}
	if e.State != 7 {
		t.Errorf("State: got %d, want 7", e.State)
	}
	if e.Total != 60000 {
		t.Errorf("Total: got %d, want 60000", e.Total)
	}
	if e.Left != 30000 {
		t.Errorf("Left: got %d, want 30000", e.Left)
	}
	if e.Val1 != 10 {
		t.Errorf("Val1: got %d, want 10", e.Val1)
	}
}

// TestActorStatusActive_0x0196_Unchanged: existing decoder still zeros timer fields.
func TestActorStatusActive_0x0196_TimerFieldsZero(t *testing.T) {
	b := make([]byte, 9)
	binary.LittleEndian.PutUint16(b[0:], 0x0196)
	binary.LittleEndian.PutUint16(b[2:], 3)  // index
	binary.LittleEndian.PutUint32(b[4:], 99) // AID
	b[8] = 5                                 // state

	e := ActorStatusActive_0x0196(b, 20060101)

	if e.State != 5 {
		t.Errorf("State: got %d, want 5", e.State)
	}
	if e.Total != 0 || e.Left != 0 || e.Val1 != 0 {
		t.Errorf("Timer fields should be zero for 0x0196, got Total=%d Left=%d Val1=%d",
			e.Total, e.Left, e.Val1)
	}
}

// ── Actor middle-gen (spot-check one per action) ──────────────────────────────

// TestActorExists_0x0915_Golden: pv=20120221 era, 74-byte packet, includes maxHP+HP+isBoss.
func TestActorExists_0x0915_Golden(t *testing.T) {
	b := make([]byte, 74)
	binary.LittleEndian.PutUint16(b[0:], 0x0915)
	binary.LittleEndian.PutUint16(b[2:], 74)         // PacketLength
	b[4] = 1                                         // objecttype (PC)
	binary.LittleEndian.PutUint32(b[5:], 0x12345678) // GID
	binary.LittleEndian.PutUint16(b[9:], 150)        // speed
	// ... fill rest with zeros except the fields we test
	binary.LittleEndian.PutUint16(b[37:], 3)      // headDir
	binary.LittleEndian.PutUint16(b[39:], 0x0010) // robe (offset 39 at this pv)
	binary.LittleEndian.PutUint32(b[65:], 1000)   // maxHP (offset 65)
	binary.LittleEndian.PutUint32(b[69:], 750)    // HP (offset 69)
	b[73] = 0                                     // isBoss

	e := ActorExists_0x0915(b, 20120221)

	if e.GID != 0x12345678 {
		t.Errorf("GID: got %#x, want 0x12345678", e.GID)
	}
	if e.Speed != 150 {
		t.Errorf("Speed: got %d, want 150", e.Speed)
	}
	if e.Robe != 0x0010 {
		t.Errorf("Robe: got %#x, want 0x0010", e.Robe)
	}
	if e.MaxHP != 1000 {
		t.Errorf("MaxHP: got %d, want 1000", e.MaxHP)
	}
	if e.HP != 750 {
		t.Errorf("HP: got %d, want 750", e.HP)
	}
}

// TestActorConnected_0x07F8_Golden: pv=20091103 era, 62-byte packet, no robe.
func TestActorConnected_0x07F8_Golden(t *testing.T) {
	b := make([]byte, 62)
	binary.LittleEndian.PutUint16(b[0:], 0x07F8)
	binary.LittleEndian.PutUint16(b[2:], 62)         // PacketLength
	b[4] = 5                                         // objecttype (NPC)
	binary.LittleEndian.PutUint32(b[5:], 0xABCD1234) // GID
	binary.LittleEndian.PutUint16(b[9:], 200)        // speed
	// PosDir at offset 53
	b[53] = 0xA4 // sample PosDir byte 0
	b[54] = 0x08
	b[55] = 0x10

	e := ActorConnected_0x07F8(b, 20091103)

	if e.GID != 0xABCD1234 {
		t.Errorf("GID: got %#x, want 0xABCD1234", e.GID)
	}
	if e.Speed != 200 {
		t.Errorf("Speed: got %d, want 200", e.Speed)
	}
	if e.Robe != 0 {
		t.Errorf("Robe should be 0 (no robe at this pv), got %d", e.Robe)
	}
	if e.PosDir[0] != 0xA4 {
		t.Errorf("PosDir[0]: got %#x, want 0xA4", e.PosDir[0])
	}
}

// TestActorMoved_0x0856_Golden: pv=20101124 era, 71-byte packet, has robe.
func TestActorMoved_0x0856_Golden(t *testing.T) {
	b := make([]byte, 71)
	binary.LittleEndian.PutUint16(b[0:], 0x0856)
	binary.LittleEndian.PutUint16(b[2:], 71)          // PacketLength
	b[4] = 1                                          // objecttype
	binary.LittleEndian.PutUint32(b[5:], 0x55556666)  // GID
	binary.LittleEndian.PutUint16(b[9:], 175)         // speed
	binary.LittleEndian.PutUint32(b[29:], 0xDEAD0000) // moveStartTime
	binary.LittleEndian.PutUint16(b[43:], 0x0020)     // robe (offset 43 at this pv)
	// MoveData at offset 59
	b[59] = 0x11
	b[60] = 0x22
	b[61] = 0x33
	b[62] = 0x44
	b[63] = 0x55
	b[64] = 0x66

	e := ActorMoved_0x0856(b, 20101124)

	if e.GID != 0x55556666 {
		t.Errorf("GID: got %#x, want 0x55556666", e.GID)
	}
	if e.MoveStartTime != 0xDEAD0000 {
		t.Errorf("MoveStartTime: got %#x, want 0xDEAD0000", e.MoveStartTime)
	}
	if e.Robe != 0x0020 {
		t.Errorf("Robe: got %#x, want 0x0020", e.Robe)
	}
	if e.MoveData[0] != 0x11 || e.MoveData[5] != 0x66 {
		t.Errorf("MoveData: got %v, want [0x11 ... 0x66]", e.MoveData)
	}
}
