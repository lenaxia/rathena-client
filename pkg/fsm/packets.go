package fsm

// copyStr copies s into dst as a null-terminated C string, truncating if needed.
// It always zero-fills the remainder of dst.
func copyStr(dst []byte, s string) {
	n := copy(dst, s)
	for i := n; i < len(dst); i++ {
		dst[i] = 0
	}
}
