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
// Offending IDs discovered so far are frozen in knownLengthOverread below so
// the suite stays green while the codegen fix (evaluate struct guards at the
// mapping's packetver_range, not the session packetver) lands. A NEW offender
// fails this test — the list must shrink, never grow. When the codegen fix
// lands, empty the list and delete the skip.
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
var knownLengthOverread = map[uint16]string{
	0x0069: "variable: decoder reads past length-table frame",
	0x006A: "fixed: decoder reads past length-table frame",
	0x006B: "variable: decoder reads past length-table frame",
	0x0079: "fixed: decoder reads past length-table frame",
	0x007B: "fixed: decoder reads past length-table frame",
	0x008D: "variable: decoder reads past length-table frame",
	0x009E: "fixed: decoder reads past length-table frame",
	0x00B4: "variable: decoder reads past length-table frame",
	0x00B7: "variable: decoder reads past length-table frame",
	0x0109: "variable: decoder reads past length-table frame",
	0x0114: "fixed: decoder reads past length-table frame",
	0x0119: "fixed: decoder reads past length-table frame",
	0x011F: "fixed: decoder reads past length-table frame",
	0x0136: "variable: decoder reads past length-table frame",
	0x0152: "variable: decoder reads past length-table frame",
	0x0162: "variable: decoder reads past length-table frame",
	0x0166: "variable: decoder reads past length-table frame",
	0x01C3: "variable: decoder reads past length-table frame",
	0x01D8: "fixed: decoder reads past length-table frame",
	0x01D9: "fixed: decoder reads past length-table frame",
	0x01DA: "fixed: decoder reads past length-table frame",
	0x022E: "fixed: decoder reads past length-table frame",
	0x025A: "variable: decoder reads past length-table frame",
	0x0284: "fixed: decoder reads past length-table frame",
	0x02B9: "fixed: decoder reads past length-table frame",
	0x0442: "variable: decoder reads past length-table frame",
	0x07F7: "variable: decoder reads past length-table frame",
	0x07F8: "variable: decoder reads past length-table frame",
	0x07F9: "variable: decoder reads past length-table frame",
	0x0836: "variable: decoder reads past length-table frame",
	0x0856: "variable: decoder reads past length-table frame",
	0x0857: "variable: decoder reads past length-table frame",
	0x0858: "variable: decoder reads past length-table frame",
	0x08C0: "variable: decoder reads past length-table frame",
	0x090F: "variable: decoder reads past length-table frame",
	0x0914: "variable: decoder reads past length-table frame",
	0x0915: "variable: decoder reads past length-table frame",
	0x09D7: "variable: decoder reads past length-table frame",
	0x09DA: "variable: decoder reads past length-table frame",
	0x09DB: "variable: decoder reads past length-table frame",
	0x09DC: "variable: decoder reads past length-table frame",
	0x09DD: "variable: decoder reads past length-table frame",
	0x09DE: "variable: decoder reads past length-table frame",
	0x09EB: "variable: decoder reads past length-table frame",
	0x09FD: "variable: decoder reads past length-table frame",
	0x09FE: "variable: decoder reads past length-table frame",
	0x09FF: "variable: decoder reads past length-table frame",
	0x0A3B: "variable: decoder reads past length-table frame",
	0x0A59: "variable: decoder reads past length-table frame",
	0x0A6B: "variable: decoder reads past length-table frame",
	0x0AA2: "variable: decoder reads past length-table frame",
	0x0AC4: "variable: decoder reads past length-table frame",
	0x0ADB: "variable: decoder reads past length-table frame",
	0x0B03: "variable: decoder reads past length-table frame",
	0x0B08: "variable: decoder reads past length-table frame",
	0x0B09: "variable: decoder reads past length-table frame",
	0x0B0A: "variable: decoder reads past length-table frame",
	0x0B8D: "variable: decoder reads past length-table frame",
}

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
