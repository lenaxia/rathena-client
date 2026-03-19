// gen-fixture parses OpenKore hex-dump files and writes binary .fixture files
// for use by the replay integration tests in pkg/session/fsm_replay_test.go.
//
// Usage:
//
//	go run ./cmd/gen-fixture -dump <path> -out <path.fixture> -packetver <YYYYMMDD>
//
// The dump format is the OpenKore "recvpackets.txt" hex-dump format:
//
//	>> Sent packet: XXXX  [Name] [N bytes]   YYYY.MM.DD HH:MM:SS
//	  0>  HH HH HH HH ...
//	<< Received packet: XXXX - Name [N bytes]   YYYY.MM.DD HH:MM:SS
//	  0>  HH HH HH HH ...
//
// Phase transitions are identified by C→S boundary packets:
//   - Login phase ends when >> Sent 0x0065 (CH_ENTER) is seen
//   - Char phase ends when >> Sent 0x0436 (CZ_ENTER) is seen
//   - Map phase: all remaining S→C bytes after 0x0436
//
// Fixture file format (binary):
//
//	[4 bytes: magic "RATF"]
//	[4 bytes: uint32 LE version = 1]
//	[4 bytes: uint32 LE packetver]
//	[repeated phase blocks:]
//	  [1 byte: phase tag 0x01=login 0x02=char 0x03=map]
//	  [4 bytes: uint32 LE byte count N]
//	  [N bytes: raw S→C bytes for this phase]
//	[4 bytes: magic "END "]
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	phaseLogin = 0x01
	phaseChar  = 0x02
	phaseMap   = 0x03
)

func main() {
	dumpPath := flag.String("dump", "", "path to OpenKore hex-dump file (required)")
	outPath := flag.String("out", "", "output .fixture file path (required)")
	packetver := flag.Uint("packetver", 20200401, "PACKETVER as YYYYMMDD integer")
	flag.Parse()

	if *dumpPath == "" || *outPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	phases, err := parseDump(*dumpPath)
	if err != nil {
		log.Fatalf("parse dump: %v", err)
	}

	if err := writeFixture(*outPath, uint32(*packetver), phases); err != nil {
		log.Fatalf("write fixture: %v", err)
	}

	log.Printf("wrote %s: login=%d bytes, char=%d bytes, map=%d bytes",
		*outPath, len(phases[0]), len(phases[1]), len(phases[2]))
}

// parsedPhases holds [login, char, map] S→C byte slices.
type parsedPhases [3][]byte

// parseDump reads an OpenKore hex-dump and splits S→C bytes into three phases.
// Phase boundaries are the C→S packets 0x0065 (login→char) and 0x0436 (char→map).
func parseDump(path string) (parsedPhases, error) {
	f, err := os.Open(path)
	if err != nil {
		return parsedPhases{}, err
	}
	defer f.Close()

	var phases parsedPhases
	// phase: 0=login, 1=char, 2=map
	currentPhase := 0

	type blockState int
	const (
		stateNone blockState = iota
		stateSend            // reading a >> Sent block
		stateRecv            // reading a << Received block
	)

	state := stateNone
	isRecv := false // current block is a received (S→C) packet

	scanner := bufio.NewScanner(f)
	// Increase buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var currentBytes []byte
	var currentPacketID uint16

	flushBlock := func() {
		if state == stateRecv && len(currentBytes) > 0 {
			phases[currentPhase] = append(phases[currentPhase], currentBytes...)
		}
		currentBytes = nil
		currentPacketID = 0
		isRecv = false
		state = stateNone
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect packet header lines
		if strings.HasPrefix(trimmed, ">> Sent packet:") {
			flushBlock()
			state = stateSend
			id, ok := extractPacketID(trimmed)
			if !ok {
				state = stateNone
				continue
			}
			currentPacketID = id
			isRecv = false

			// Phase transition boundaries
			switch id {
			case 0x0065: // CH_ENTER — login→char
				if currentPhase == 0 {
					currentPhase = 1
				}
			case 0x0436: // CZ_ENTER — char→map
				if currentPhase == 1 {
					currentPhase = 2
				}
			}
			continue
		}

		if strings.HasPrefix(trimmed, "<< Received packet:") {
			flushBlock()
			state = stateRecv
			isRecv = true
			id, ok := extractPacketID(trimmed)
			if !ok {
				state = stateNone
				continue
			}
			currentPacketID = id
			_ = currentPacketID
			continue
		}

		// Hex data lines: "  N>  HH HH HH ..."
		if state != stateNone && isHexDataLine(trimmed) {
			b := parseHexLine(trimmed)
			if isRecv {
				currentBytes = append(currentBytes, b...)
			}
			continue
		}

		// Any non-data line (annotation, separator) that isn't a new block header:
		// if we were reading hex data, flush on "===" separator or blank line after data
		if state != stateNone && (trimmed == strings.Repeat("=", len(trimmed)) || trimmed == "") {
			// Don't flush yet — let next block header trigger flush
		}
		_ = isRecv
	}
	flushBlock()

	if err := scanner.Err(); err != nil {
		return parsedPhases{}, fmt.Errorf("scanner: %w", err)
	}

	return phases, nil
}

// extractPacketID parses the 4-hex-digit packet ID from a dump header line.
// Examples:
//
//	">> Sent packet: 0065  [Character Server Login] [17 bytes]"   → 0x0065
//	"<< Received packet:      0AC4 - Account Info [224 bytes]"    → 0x0AC4
func extractPacketID(line string) (uint16, bool) {
	// Find the first 4-hex sequence after "packet:" or "packet:"
	idx := strings.Index(line, "packet:")
	if idx < 0 {
		return 0, false
	}
	rest := strings.TrimSpace(line[idx+7:])
	// rest now starts with the hex ID
	if len(rest) < 4 {
		return 0, false
	}
	b, err := hex.DecodeString(rest[:4])
	if err != nil || len(b) != 2 {
		return 0, false
	}
	return uint16(b[0])<<8 | uint16(b[1]), true
}

// isHexDataLine returns true for lines that contain hex dump data.
// Pattern: optional whitespace, then digits, then ">  HH HH ..."
func isHexDataLine(s string) bool {
	// Must contain ">" and hex-looking content
	idx := strings.Index(s, ">")
	if idx < 0 {
		return false
	}
	after := strings.TrimSpace(s[idx+1:])
	if len(after) == 0 {
		return false
	}
	// Check first token looks like hex
	parts := strings.Fields(after)
	if len(parts) == 0 {
		return false
	}
	// Each hex token is exactly 2 chars
	for _, p := range parts {
		if len(p) == 2 {
			_, err := hex.DecodeString(p)
			if err == nil {
				return true
			}
		}
	}
	return false
}

// parseHexLine extracts the hex bytes from a dump data line.
// Format: "  0>  HH HH HH HH    HH HH HH HH    ASCII..."
// We take all 2-char hex tokens before the ASCII column.
func parseHexLine(s string) []byte {
	// Find the ">" separator
	idx := strings.Index(s, ">")
	if idx < 0 {
		return nil
	}
	rest := strings.TrimSpace(s[idx+1:])

	// The hex section and ASCII section are separated by multiple spaces.
	// We parse tokens that are exactly 2 hex chars; stop at anything else
	// (ASCII column appears as printable chars, not 2-char hex).
	var result []byte
	fields := strings.Fields(rest)
	for _, f := range fields {
		if len(f) == 2 {
			b, err := hex.DecodeString(f)
			if err == nil {
				result = append(result, b...)
			} else {
				// Not hex — hit ASCII column, stop
				break
			}
		} else {
			// Not a 2-char token — skip (could be ASCII or separator)
			break
		}
	}
	return result
}

// writeFixture writes a binary .fixture file from the three phase byte slices.
func writeFixture(path string, packetver uint32, phases parsedPhases) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	// Magic header
	if _, err := w.WriteString("RATF"); err != nil {
		return err
	}
	// Version = 1
	if err := binary.Write(w, binary.LittleEndian, uint32(1)); err != nil {
		return err
	}
	// Packetver
	if err := binary.Write(w, binary.LittleEndian, packetver); err != nil {
		return err
	}

	// Phase blocks
	tags := [3]byte{phaseLogin, phaseChar, phaseMap}
	for i, tag := range tags {
		data := phases[i]
		if err := w.WriteByte(tag); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(len(data))); err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}

	// End magic
	if _, err := w.WriteString("END "); err != nil {
		return err
	}

	return w.Flush()
}
