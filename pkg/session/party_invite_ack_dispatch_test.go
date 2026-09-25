// Dispatch tests for ZC party invite-ack (goKore bug report in worklog
// 1248 §R4, issue #385 / PR #421). Each test feeds a wire-valid frame and
// asserts the ActionZcPartyJoinReqAck handler fires with the decoded event.
// Companion golden-byte decode tests: pkg/decode/zc_party_join_req_ack_test.go.

package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

func TestZcPartyJoinReqAck_0x02C5_FiresAtPv20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x02C5]; got != 30 {
		t.Fatalf("prerequisite: lengths[0x02C5] = %d at pv=%d, want 30", got, pv)
	}

	fired := 0
	var gotEvent events.ZcPartyJoinReqAck
	RegisterSemanticHandler(s, ActionZcPartyJoinReqAck, func(e events.ZcPartyJoinReqAck) {
		fired++
		gotEvent = e
	})

	buf := make([]byte, 30)
	binary.LittleEndian.PutUint16(buf[0:2], 0x02C5)
	copy(buf[2:26], "Bob")
	binary.LittleEndian.PutUint32(buf[26:30], 2) // PARTY_REPLY_ACCEPTED

	if err := s.Feed(buf); err != nil {
		t.Fatalf("Feed(0x02C5) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcPartyJoinReqAck fired %d times for 0x02C5, want 1", fired)
	}
	if gotEvent.CharacterName != "Bob" {
		t.Errorf("CharacterName: got %q, want Bob", gotEvent.CharacterName)
	}
	if gotEvent.Result != 2 {
		t.Errorf("Result: got %d, want 2 (accepted)", gotEvent.Result)
	}
}

func TestZcPartyJoinReqAck_0x00FD_FiresAtLegacyPv(t *testing.T) {
	pv := uint32(20070820)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x00FD]; got != 30 {
		t.Fatalf("prerequisite: lengths[0x00FD] = %d at pv=%d, want 30 (override applied)", got, pv)
	}

	fired := 0
	var gotEvent events.ZcPartyJoinReqAck
	RegisterSemanticHandler(s, ActionZcPartyJoinReqAck, func(e events.ZcPartyJoinReqAck) {
		fired++
		gotEvent = e
	})

	buf := make([]byte, 30)
	binary.LittleEndian.PutUint16(buf[0:2], 0x00FD)
	copy(buf[2:26], "Alice")
	binary.LittleEndian.PutUint32(buf[26:30], 258) // 0x0102 — above uint8 range, pins the int32 result

	if err := s.Feed(buf); err != nil {
		t.Fatalf("Feed(0x00FD) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcPartyJoinReqAck fired %d times for 0x00FD, want 1", fired)
	}
	if gotEvent.CharacterName != "Alice" {
		t.Errorf("CharacterName: got %q, want Alice", gotEvent.CharacterName)
	}
	if gotEvent.Result != 258 {
		t.Errorf("Result: got %d, want 258", gotEvent.Result)
	}
}

// TestZcPartyJoinReqAck_CrossPacketverLegacyFiresAtModernPv pins the documented
// "registered at all packetvers — never fire in practice, harmless" dispatch
// contract: a 0x00FD frame fed to a modern-pv session still fires the action
// through the legacy-ID decoder, and the override keeps the framer in sync
// (30-byte consumption, no stream desync).
func TestZcPartyJoinReqAck_CrossPacketverLegacyFiresAtModernPv(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x00FD]; got != 30 {
		t.Fatalf("prerequisite: lengths[0x00FD] = %d at pv=%d, want 30 (override applied)", got, pv)
	}

	fired := 0
	var gotEvent events.ZcPartyJoinReqAck
	RegisterSemanticHandler(s, ActionZcPartyJoinReqAck, func(e events.ZcPartyJoinReqAck) {
		fired++
		gotEvent = e
	})

	buf := make([]byte, 30)
	binary.LittleEndian.PutUint16(buf[0:2], 0x00FD)
	copy(buf[2:26], "Mallory")
	binary.LittleEndian.PutUint32(buf[26:30], 5) // blocked

	if err := s.Feed(buf); err != nil {
		t.Fatalf("Feed(0x00FD) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcPartyJoinReqAck fired %d times for 0x00FD at pv=%d, want 1", fired, pv)
	}
	if gotEvent.CharacterName != "Mallory" {
		t.Errorf("CharacterName: got %q, want Mallory", gotEvent.CharacterName)
	}
	if gotEvent.Result != 5 {
		t.Errorf("Result: got %d, want 5", gotEvent.Result)
	}
}
