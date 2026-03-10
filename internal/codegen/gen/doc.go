// Package gen contains code generators that emit Go source files from the
// parsed rAthena packet structures and semantic mappings.
package gen

import "strings"

// isSendStruct reports whether a struct name corresponds to a client-to-server packet
// (PACKET_CZ_*, PACKET_CH_*, PACKET_CA_*, or any SYNTH_CZ_*/SYNTH_CH_*/SYNTH_CA_* variant).
func isSendStruct(name string) bool {
	return strings.HasPrefix(name, "PACKET_CZ_") ||
		strings.HasPrefix(name, "PACKET_CH_") ||
		strings.HasPrefix(name, "PACKET_CA_") ||
		strings.HasPrefix(name, "SYNTH_CZ_") ||
		strings.HasPrefix(name, "SYNTH_CH_") ||
		strings.HasPrefix(name, "SYNTH_CA_")
}
