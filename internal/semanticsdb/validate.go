package semanticsdb

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ValidationError is one issue reported by Validate. ActionName/PacketID may
// be empty for DB-wide errors.
type ValidationError struct {
	ActionName string
	PacketID   string
	Message    string
}

func (e ValidationError) Error() string {
	var loc string
	switch {
	case e.ActionName != "" && e.PacketID != "":
		loc = fmt.Sprintf("action=%s packet=%s", e.ActionName, e.PacketID)
	case e.ActionName != "":
		loc = fmt.Sprintf("action=%s", e.ActionName)
	case e.PacketID != "":
		loc = fmt.Sprintf("packet=%s", e.PacketID)
	default:
		loc = "(db-wide)"
	}
	return loc + ": " + e.Message
}

var packetIDRe = regexp.MustCompile(`^0x[0-9A-Fa-f]{4}$`)

// Validate runs a set of structural checks against the DB and returns all
// errors found. Returns nil if the DB is consistent.
//
// Checks performed:
//
//   - Action names are non-empty and match the lowercase_snake_case pattern.
//   - Every implementation has a non-empty packet_id and struct_name.
//   - packet_id values match /^0x[0-9A-Fa-f]{4}$/.
//   - packet_id values are unique within an action (no duplicate impls).
//   - packetver ranges satisfy min <= max when both are non-zero.
//   - No two actions share the same packet_id unless their packetver ranges
//     are disjoint (cross-action conflict detection).
func (d *DB) Validate() []ValidationError {
	var errs []ValidationError

	// Per-action checks.
	for _, name := range d.ListActions() {
		a, _ := d.GetAction(name)
		if !isValidActionName(name) {
			errs = append(errs, ValidationError{
				ActionName: name,
				Message:    "action name must be lowercase_snake_case (letters, digits, underscores; must start with a letter)",
			})
		}

		seenPackets := make(map[string]bool, len(a.Implementations))
		for _, impl := range a.Implementations {
			base := ValidationError{ActionName: name, PacketID: impl.PacketID}
			if impl.PacketID == "" {
				errs = append(errs, ValidationError{
					ActionName: name,
					Message:    "implementation has empty packet_id",
				})
				continue
			}
			if !packetIDRe.MatchString(impl.PacketID) {
				errs = append(errs, base)
				errs[len(errs)-1].Message = fmt.Sprintf("packet_id %q does not match /^0x[0-9A-Fa-f]{4}$/", impl.PacketID)
			}
			if impl.StructName == "" {
				errs = append(errs, base)
				errs[len(errs)-1].Message = "implementation has empty struct_name"
			}
			if seenPackets[impl.PacketID] {
				errs = append(errs, base)
				errs[len(errs)-1].Message = fmt.Sprintf("duplicate implementation for packet_id %s within action", impl.PacketID)
			}
			seenPackets[impl.PacketID] = true
			if impl.PacketverMin != 0 && impl.PacketverMax != 0 && impl.PacketverMin > impl.PacketverMax {
				errs = append(errs, base)
				errs[len(errs)-1].Message = fmt.Sprintf("packetver_range min (%d) > max (%d)", impl.PacketverMin, impl.PacketverMax)
			}
		}
	}

	// Cross-action conflict check: same packet_id must have disjoint
	// packetver ranges (or be unbounded in only one place).
	errs = append(errs, d.validateCrossActionConflicts()...)

	sort.Slice(errs, func(i, j int) bool {
		if errs[i].ActionName != errs[j].ActionName {
			return errs[i].ActionName < errs[j].ActionName
		}
		return errs[i].PacketID < errs[j].PacketID
	})

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// validateCrossActionConflicts flags packet_id reuse across actions where the
// packetver ranges overlap. Two actions legitimately sharing a packet_id
// (e.g. 0x0092 used by two different actions) must have disjoint ranges.
func (d *DB) validateCrossActionConflicts() []ValidationError {
	type usage struct {
		action string
		min    int
		max    int
	}
	byPacket := make(map[string][]usage)
	for _, name := range d.ListActions() {
		a, _ := d.GetAction(name)
		for _, impl := range a.Implementations {
			if impl.PacketID == "" || impl.StructName == "" {
				continue
			}
			byPacket[impl.PacketID] = append(byPacket[impl.PacketID], usage{
				action: name, min: impl.PacketverMin, max: impl.PacketverMax,
			})
		}
	}

	var errs []ValidationError
	packets := make([]string, 0, len(byPacket))
	for p := range byPacket {
		packets = append(packets, p)
	}
	sort.Strings(packets)
	for _, pid := range packets {
		us := byPacket[pid]
		if len(us) < 2 {
			continue
		}
		for i := 0; i < len(us); i++ {
			for j := i + 1; j < len(us); j++ {
				if rangesOverlap(us[i].min, us[i].max, us[j].min, us[j].max) {
					errs = append(errs, ValidationError{
						ActionName: us[i].action,
						PacketID:   pid,
						Message: fmt.Sprintf("packet_id %s used by both action %s and %s with overlapping packetver ranges",
							pid, us[i].action, us[j].action),
					})
				}
			}
		}
	}
	return errs
}

// rangesOverlap reports whether [aMin, aMax] overlaps [bMin, bMax]. Zero
// means "unbounded" on that side. Packetver values are positive YYYYMMDD
// integers, so 0 is below any real value and can be used as the lower
// sentinel; the upper sentinel uses a large int to model "infinity".
func rangesOverlap(aMin, aMax, bMin, bMax int) bool {
	const inf = 1 << 62
	aLo := aMin
	if aMin == 0 {
		aLo = 0
	}
	aHi := aMax
	if aMax == 0 {
		aHi = inf
	}
	bLo := bMin
	if bMin == 0 {
		bLo = 0
	}
	bHi := bMax
	if bMax == 0 {
		bHi = inf
	}
	return aLo <= bHi && bLo <= aHi
}

var actionNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func isValidActionName(s string) bool {
	return actionNameRe.MatchString(s)
}

// FormatErrors formats a slice of ValidationError as a single multi-line
// string suitable for CLI/MCP output.
func FormatErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return "OK: no validation errors"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d validation error(s):\n", len(errs))
	for _, e := range errs {
		fmt.Fprintf(&b, "  - %s\n", e.Error())
	}
	return b.String()
}
