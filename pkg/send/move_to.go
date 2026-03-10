// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-16.
// This deviates from the SemanticDB canonical params (Coords [3]byte) to provide
// caller-friendly X/Y coordinates. packing.EncodePosDir is called inside EncodeMoveTo.

package send

// MoveTo is the C→S request struct for the move_to action.
// X and Y are the destination tile coordinates in map units.
type MoveTo struct {
	X uint16
	Y uint16
}
