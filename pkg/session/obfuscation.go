// Hand-written: obfuscationState.
package session

// obfuscationState tracks the C→S packet ID obfuscation rolling key.
// Only used by MapSession. S→C packets received via Feed are never obfuscated.
//
// Source: src/map/clif_obfuscation.hpp + clif.cpp:25692–25764 (clif_parse),
//
//	clif.cpp:10721 (rolling key init).
type obfuscationState struct {
	enabled    bool
	firstSent  bool
	firstKey   uint16 // XOR key for the first C→S packet
	rollingKey uint32 // LCG state; advances after each C→S packet (after the first)
	key0       uint32 // clif_cryptKey[0]
	key1       uint32 // clif_cryptKey[1] — LCG multiplier
	key2       uint32 // clif_cryptKey[2] — LCG addend
}
