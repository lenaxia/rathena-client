// Receive-dispatch length audit.
//
// For every (action, packet ID) entry registered in receiveDispatch, the
// generated decoder must be safe when handed exactly the frame length the
// generated length table assigns to that ID: any read past the frame end
// panics with index-out-of-range, which recover() converts into an audit
// finding.
//
// This test exists because of a systemic codegen bug found by
// FuzzFeedMapSessionWithDispatch (corpus 75869bcf4b578ee9, 2026-09-29):
// decoders for legacy packet-ID variants are compiled from the struct guards
// evaluated at a modern packetver, while the length table keeps rAthena's
// legacy clif_packetdb.hpp size for those IDs. Example: actor_connected
// 0x01D9 is scoped to pv 2003–2005 in semantics/mappings.yaml with wire
// length 53, but ActorConnected_0x01D9 reads through offset 55; 0x0079 reads
// through offset 107 against length 53.
//
// The list is EMPTY: every generated decode branch emits a length guard
// (see gen/decode.go layoutMinLen) and hand-written decoders carry entry
// guards, so a short frame decodes to a zero event instead of panicking.
// A NEW offender fails this test — triage it, fix it, keep the list empty.
//
// Runtime risk note: all known offenders are legacy IDs that rAthena does not
// send at modern packetvers, which is why live captures never tripped them.
package session

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// knownLengthOverread lists receiveDispatch packet IDs whose generated decoder
// reads past the length-table frame for the ID. Each entry keeps CI green
// while the fix is pending; the goal is for this map to be empty.
// knownLengthOverread lists receiveDispatch packet IDs whose decoder still
// reads past the length-table frame. EMPTY since the BUG-02/BUG-03 fixes:
// generated decoders emit per-layout length guards and evaluate legacy
// variants at their era layout; hand-written decoders carry entry guards.
// A NEW entry here fails CI — triage, fix, and empty the list again.
var knownLengthOverread = map[uint16]string{}

// auditResult is one over-read finding: packet ID and its semantic action.
type auditResult struct {
	id     uint16
	action SemanticAction
}

func TestReceiveDispatchLengthAudit(t *testing.T) {
	s := NewMapSession(20200401)

	var offenders []auditResult

	for action, entries := range receiveDispatch {
		for _, e := range entries {
			entry := e
			frameLen := int(s.core.lengths[entry.id])
			if frameLen == 0 {
				// Unknown to the length table: the framer clears the stream
				// before any decoder runs. Nothing to audit.
				continue
			}
			if frameLen == -1 {
				// Variable-length: the framer accepts any embedded length ≥ 4,
				// so a decoder handed a minimal 4-byte frame must not read
				// past it. Found live by fuzz corpus 061fce683bbdf652
				// (AcAcceptLogin_0x0AC4, index-out-of-range at offset 46).
				frame := []byte{byte(entry.id), byte(entry.id >> 8), 0x04, 0x00}
				if decodePanics(frame, entry.fn) {
					offenders = append(offenders, auditResult{entry.id, action})
				}
				continue
			}
			frame := make([]byte, frameLen)
			for i := range frame {
				frame[i] = 0xA5
			}
			if decodePanics(frame, entry.fn) {
				offenders = append(offenders, auditResult{entry.id, action})
			}
		}
	}

	sort.Slice(offenders, func(i, j int) bool {
		if offenders[i].id != offenders[j].id {
			return offenders[i].id < offenders[j].id
		}
		return offenders[i].action < offenders[j].action
	})

	var fresh []string
	for _, o := range offenders {
		if _, known := knownLengthOverread[o.id]; !known {
			fresh = append(fresh, fmt.Sprintf("0x%04X (%v)", o.id, o.action))
		}
	}
	if len(fresh) > 0 {
		t.Errorf("NEW receive-dispatch length over-reads (decoder reads past length-table frame):\n  %s\n"+
			"Fix the codegen/semantics for these IDs, or add them to knownLengthOverread if triaged.",
			strings.Join(fresh, "\n  "))
	}

	// Report stale entries in the known list so they get removed after fixes.
	seen := make(map[uint16]bool, len(offenders))
	for _, o := range offenders {
		seen[o.id] = true
	}
	var stale []string
	for id := range knownLengthOverread {
		if !seen[id] {
			stale = append(stale, fmt.Sprintf("0x%04X no longer over-reads — remove from knownLengthOverread", id))
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Logf("stale known entries:\n  %s", strings.Join(stale, "\n  "))
	}

	t.Logf("audit: %d over-reads (all known): %s", len(offenders), formatIDs(offenders))
}

// decodePanics calls fn on frame in a goroutine with recover, reporting
// whether the call panicked.
func decodePanics(frame []byte, fn func([]byte, uint32) interface{}) (panicked bool) {
	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
			done <- true
		}()
		_ = fn(frame, 20200401)
	}()
	<-done
	return panicked
}

func formatIDs(rs []auditResult) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("0x%04X(%v)", r.id, r.action)
	}
	return strings.Join(parts, " ")
}
