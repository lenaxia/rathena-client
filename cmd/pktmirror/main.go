// Command pktmirror is a passive TCP mirror logger for rAthena protocol
// captures. It sits between a client (goKore, a test, or the FSM integration
// test) and the real rAthena server trio, logging both directions of every
// connection and — in trio mode — emitting a .fixture file directly usable by
// pkg/session replay tests and cmd/gen-fixture workflows.
//
// Modes of operation:
//
// Single mode (one connection pair):
//
//	pktmirror -listen 127.0.0.1:16900 -upstream 127.0.0.1:6900
//
// Trio mode (login/char/map proxy in one process). When -packetver is set,
// the S→C bytes of each phase are additionally written as a RATF v1 .fixture
// file (same format as pkg/session/testdata/*.fixture):
//
//	pktmirror -trio \
//	  -login-listen  127.0.0.1:6900 -login-upstream  127.0.0.1:36900 \
//	  -char-listen   127.0.0.1:6121 -char-upstream   127.0.0.1:36121 \
//	  -map-listen    127.0.0.1:5121 -map-upstream    127.0.0.1:35121 \
//	  -packetver 20200401 -label mycapture
//
// Trio mode relies on rAthena advertising the mirror's listening addresses to
// the client (char_ip / map_ip conf). See hack/testserver/README.md for the
// exact docker-compose topology that makes this work without packet rewriting.
//
// Output files (in -out, default ./captures):
//
//	<label>_<phase>_<ts>.log    hexdump log, both directions, timestamped
//	<label>_<phase>_<ts>_c2s.bin raw client→server byte stream
//	<label>_<phase>_<ts>_s2c.bin raw server→client byte stream
//	<label>_<ts>.fixture         RATF v1 fixture (trio mode + -packetver only)
//
// The fixture contains only S→C bytes (replay tests script server responses).
package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"
)

const fixtureMagic = "RATF"

type phaseStats struct {
	label string

	mu        sync.Mutex
	c2s, s2c  []byte
	conns     int
	firstSeen time.Time
}

func newPhaseStats(label string) *phaseStats {
	return &phaseStats{label: label, firstSeen: time.Now()}
}

func (p *phaseStats) add(dir string, b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch dir {
	case "c2s":
		p.c2s = append(p.c2s, b...)
	case "s2c":
		p.s2c = append(p.s2c, b...)
	}
}

type mirror struct {
	out       string
	label     string
	packetver uint32
	fixture   bool
	phase     string
	stats     *phaseStats

	logFile *os.File
}

func timestamp() string { return time.Now().UTC().Format("20060102T150405Z") }

func newMirror(out, label, phase string, stats *phaseStats) (*mirror, error) {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	ts := timestamp()
	base := filepath.Join(out, fmt.Sprintf("%s_%s_%s", label, phase, ts))
	f, err := os.Create(base + ".log")
	if err != nil {
		return nil, err
	}
	return &mirror{out: out, label: label, phase: phase, stats: stats, logFile: f}, nil
}

// hexdump writes an annotated 16-bytes-per-line hexdump of b into the log.
func (m *mirror) hexdump(dir string, b []byte) {
	const perLine = 16
	fmt.Fprintf(m.logFile, "[%s] %s len=%d\n", time.Now().UTC().Format(time.RFC3339Nano), dir, len(b))
	for off := 0; off < len(b); off += perLine {
		end := off + perLine
		if end > len(b) {
			end = len(b)
		}
		line := b[off:end]
		hexPart := hex.EncodeToString(line)
		// Pad hex column so ASCII column lines up regardless of line length.
		for i := len(line); i < perLine; i++ {
			hexPart += "  "
		}
		ascii := make([]byte, 0, perLine)
		for _, c := range line {
			if c >= 0x20 && c < 0x7f {
				ascii = append(ascii, c)
			} else {
				ascii = append(ascii, '.')
			}
		}
		fmt.Fprintf(m.logFile, "  %04x  %s  |%s|\n", off, hexPart, ascii)
	}
}

// pipe copies src→dst in chunks, logging and accumulating every read.
func (m *mirror) pipe(dst net.Conn, src net.Conn, dir string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if c, ok := dst.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
	}()
	buf := make([]byte, 32*1024)
	for {
		_ = src.SetReadDeadline(time.Now().Add(24 * time.Hour))
		n, err := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			m.hexdump(dir, chunk)
			m.stats.add(dir, chunk)
			if _, werr := dst.Write(chunk); werr != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("%s/%s: %s read: %v", m.label, m.phase, dir, err)
			}
			return
		}
	}
}

// handle proxies one accepted connection to upstream.
func (m *mirror) handle(client net.Conn, upstreamAddr string) {
	defer client.Close()
	m.stats.mu.Lock()
	m.stats.conns++
	m.stats.mu.Unlock()

	up, err := net.DialTimeout("tcp", upstreamAddr, 10*time.Second)
	if err != nil {
		log.Printf("%s/%s: dial upstream %s: %v", m.label, m.phase, upstreamAddr, err)
		return
	}
	defer up.Close()

	fmt.Fprintf(m.logFile, "# connection %s <-> %s started %s\n",
		client.RemoteAddr(), upstreamAddr, time.Now().UTC().Format(time.RFC3339))

	var wg sync.WaitGroup
	wg.Add(2)
	go m.pipe(up, client, "c2s", &wg) // client → server
	go m.pipe(client, up, "s2c", &wg) // server → client
	wg.Wait()

	fmt.Fprintf(m.logFile, "# connection closed %s\n", time.Now().UTC().Format(time.RFC3339))
	_ = m.logFile.Sync()
}

// writeFixture emits a RATF v1 fixture from the captured S→C phase streams.
// Format: "RATF" + u32 version=1 + u32 packetver +
//
//	[phase-tag(1) + len(4) + data(len)] × 3 (tags 0x01 login, 0x02 char, 0x03 map) + "END "
//
// Mirrors the reader in pkg/session/fsm_scriptedserver_test.go and the writer
// in cmd/gen-fixture.
func writeFixture(path string, packetver uint32, login, char, mapPhase []byte) error {
	var buf []byte
	buf = append(buf, fixtureMagic...)
	buf = binary.LittleEndian.AppendUint32(buf, 1) // version
	buf = binary.LittleEndian.AppendUint32(buf, packetver)
	for tag, data := range [][]byte{login, char, mapPhase} {
		buf = append(buf, byte(tag+1))
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(data)))
		buf = append(buf, data...)
	}
	buf = append(buf, "END "...)
	return os.WriteFile(path, buf, 0o644)
}

func main() {
	var (
		out = flag.String("out", "captures", "output directory")

		listen       = flag.String("listen", "", "single mode: listen address")
		upstream     = flag.String("upstream", "", "single mode: upstream address")
		label        = flag.String("label", "cap", "capture label used in output file names")
		packetverU32 = flag.Uint("packetver", 0, "packetver for fixture output (0 = no fixture)")

		trio          = flag.Bool("trio", false, "trio mode: proxy login/char/map in one process")
		loginListen   = flag.String("login-listen", "127.0.0.1:6900", "trio mode: login listen address")
		loginUpstream = flag.String("login-upstream", "127.0.0.1:36900", "trio mode: login upstream address")
		charListen    = flag.String("char-listen", "127.0.0.1:6121", "trio mode: char listen address")
		charUpstream  = flag.String("char-upstream", "127.0.0.1:36121", "trio mode: char upstream address")
		mapListen     = flag.String("map-listen", "127.0.0.1:5121", "trio mode: map listen address")
		mapUpstream   = flag.String("map-upstream", "127.0.0.1:35121", "trio mode: map upstream address")
	)
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("create out dir: %v", err)
	}

	if !*trio {
		if *listen == "" || *upstream == "" {
			log.Fatal("single mode requires -listen and -upstream (or use -trio)")
		}
		stats := newPhaseStats("single")
		m, err := newMirror(*out, *label, "single", stats)
		if err != nil {
			log.Fatalf("create mirror: %v", err)
		}
		ln, err := net.Listen("tcp", *listen)
		if err != nil {
			log.Fatalf("listen %s: %v", *listen, err)
		}
		log.Printf("pktmirror single: %s -> %s (logs in %s)", *listen, *upstream, *out)
		for {
			c, err := ln.Accept()
			if err != nil {
				log.Fatalf("accept: %v", err)
			}
			go m.handle(c, *upstream)
		}
	}

	// Trio mode.
	phases := map[string]*phaseStats{
		"login": newPhaseStats("login"),
		"char":  newPhaseStats("char"),
		"map":   newPhaseStats("map"),
	}
	listeners := []struct {
		name     string
		listen   string
		upstream string
	}{
		{"login", *loginListen, *loginUpstream},
		{"char", *charListen, *charUpstream},
		{"map", *mapListen, *mapUpstream},
	}
	for _, l := range listeners {
		m, err := newMirror(*out, *label, l.name, phases[l.name])
		if err != nil {
			log.Fatalf("create mirror %s: %v", l.name, err)
		}
		ln, err := net.Listen("tcp", l.listen)
		if err != nil {
			log.Fatalf("listen %s (%s): %v", l.listen, l.name, err)
		}
		log.Printf("pktmirror trio: %s %s -> %s", l.name, l.listen, l.upstream)
		go func(m *mirror, ln net.Listener, upstreamAddr string) {
			for {
				c, err := ln.Accept()
				if err != nil {
					log.Printf("accept %s: %v", m.phase, err)
					return
				}
				go m.handle(c, upstreamAddr)
			}
		}(m, ln, l.upstream)
	}

	// On interrupt, write the fixture from whatever S→C bytes were captured.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	log.Print("shutting down; writing fixture...")
	if *packetverU32 == 0 {
		return
	}
	fixturePath := filepath.Join(*out, fmt.Sprintf("%s_%s.fixture", *label, timestamp()))
	if err := writeFixture(fixturePath, uint32(*packetverU32),
		phases["login"].s2cSnapshot(), phases["char"].s2cSnapshot(), phases["map"].s2cSnapshot()); err != nil {
		log.Fatalf("write fixture: %v", err)
	}
	log.Printf("fixture written: %s", fixturePath)
}

// s2cSnapshot returns a copy of the accumulated S→C stream.
func (p *phaseStats) s2cSnapshot() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.s2c...)
}
