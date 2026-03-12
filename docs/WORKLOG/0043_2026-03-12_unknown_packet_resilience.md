# 0043 — Unknown Packet Resilience: Clear-Buffer + UnknownPacketEvent

**Date:** 2026-03-12  
**Packages changed:** `pkg/session`  
**Status:** Complete — all tests pass, 0 allocs/op maintained

---

## What Was Done

Replaced the permanent-fault behavior on unknown packet IDs with a clear-buffer
model that matches OpenKore's `MessageTokenizer` and emits a rich diagnostic event
to the caller, including a ring buffer of the last 3 preceding dispatched packets.

### Problem

`sessionCore.feed()` previously faulted permanently on any packet ID not in the
length table (`lengths[id] == 0`). This meant a single missing entry in
`lengths_map.go` would kill the session forever with no diagnostic information —
no indication of which packet was unknown, what preceded it, or what bytes were
in the buffer.

### Design Decisions

**Why not skip 2 bytes and continue?**  
Discarding only the 2-byte ID and resuming framing is unsound. Since the unknown
packet's payload length is not known, the bytes following the ID are payload — not
the next packet header. Treating them as a packet ID risks silently dispatching
garbage to a real handler, corrupting game state with no error signal.

**Why clear the entire buffer?**  
This matches OpenKore `MessageTokenizer.readNext()` (lines 168–171):
```perl
} else {
    $result = $$buffer;
    $self->{buffer} = '';
    $$type = UNKNOWN_MESSAGE;
}
```
The entire buffer is consumed and the tokenizer resets. No silent corruption. The
next TCP read starts fresh and may recover if the server sends a known packet ID.

**Why emit an event rather than writing to a file / logger?**  
goKore runs hundreds of bots, each with exactly two goroutines (main game loop +
TCP stack). A dedicated drain goroutine, file I/O, or package-level logger in
the library would violate all three constraints. The library fires the event
synchronously in the callback (which runs on the TCP stack goroutine) and goKore's
bot manager handles it however it sees fit — logging, in-memory ring buffer,
structured telemetry, etc.

**Why store preceding packet frames rather than just IDs?**  
IDs alone tell you the sequence but not the content. Full frame bytes allow the
bot manager to fully decode what the session was doing before the stream was lost,
without needing to correlate against a separate capture.

**Why use a fixed inline backing store for the ring rather than heap-allocating per push?**  
`feed()` is the hot path — zero allocations is a hard invariant. Allocating on
every dispatch to store frames in the ring would break this. The ring uses a
`[recentPacketDepth]recentSlot` array embedded directly in `sessionCore`, where
each slot contains a `[recentMaxFrameBytes]byte` array. `push()` does a single
`copy()` into the slot — zero allocations. `snapshot()` allocates only when
building the `UnknownPacketEvent`, which is the exceptional path.

---

## Changes

### `pkg/session/session.go`

**New types:**

- `DispatchedPacket` — a record of one dispatched packet:
  - `ID uint16`
  - `Frame []byte` — heap copy of frame bytes (may be truncated to `recentMaxFrameBytes`)
  - `FrameTotal int` — actual frame length
  - `Truncated bool` — true if frame exceeded `recentMaxFrameBytes` (4096)

- `UnknownPacketEvent` — full diagnostic context:
  - `ID uint16` — the unrecognised packet ID
  - `Packetver uint32` — PACKETVER this session was constructed with
  - `Time time.Time` — wall time at moment of detection
  - `RecentPackets []DispatchedPacket` — last ≤3 dispatched packets, oldest first
  - `RawBuffer []byte` — snapshot copy of recvBuf from the unknown ID onward

- `UnknownPacketFunc` — `func(event UnknownPacketEvent)`

**New internal types:**

- `recentRing` — fixed-depth ring buffer with inline `[3]recentSlot` backing array
- `recentSlot` — single pre-allocated slot: `id uint16`, `buf [4096]byte`, `frameN int`, `frameTotal int`

**`sessionCore` changes:**

- Replaced `lastDispatchID uint16` with `recent recentRing`
- `onUnknownPacket UnknownPacketFunc` field added
- `frameLen == 0` branch: builds `UnknownPacketEvent` with `c.recent.snapshot()`,
  fires callback, clears buffer
- Step 2e now calls `c.recent.push(packetID, c.recvBuf[:frameLen])` (zero alloc)

**`ErrUnknownPacket`** is now reserved exclusively for genuine stream corruption:
a variable-length packet whose embedded length field is < 4.

### `pkg/session/map.go`, `login.go`, `char.go`

`SetUnknownPacketHandler(fn UnknownPacketFunc)` added to all three session types.

---

## goKore Integration Pattern

```go
ms.SetUnknownPacketHandler(func(ev session.UnknownPacketEvent) {
    // Runs synchronously on the TCP stack goroutine.
    // Bot manager stores or logs ev however it sees fit.
    botManager.HandleUnknownPacket(botID, ev)
})
```

`ev` is fully self-contained and heap-allocated — safe to store, pass, or log
without copying. `ev.RecentPackets[i].Frame` contains the raw bytes of each
preceding packet, ready to pass to any `decode.*` function.

---

## Memory Overhead

Per `MapSession` (steady state, no allocations on hot path):

| Component | Size |
|---|---|
| `recentRing` (3 slots × 4096 bytes) | ~12 KB |
| `sessionCore.buf` (map recv buffer) | 64 KB |
| lengths table | 128 KB |
| handlers table | ~512 KB (pointer array) |

The ring adds 12 KB per `MapSession`. At 500 bots: 6 MB total. Acceptable.

---

## Test Results

```
ok  github.com/lenaxia/rathena-client/pkg/session   0.015s
ok  github.com/lenaxia/rathena-client/pkg/fsm       0.153s
```

### Tests added / updated

| Test | What it proves |
|---|---|
| `TestMapSession_Feed_UnknownPacket` | Full event fields correct; RecentPackets contains preceding packet; buffer cleared; session recovers next Feed |
| `TestMapSession_Feed_UnknownPacket_NoCallback` | Nil callback: silent clear, no panic, no fault |
| `TestMapSession_Feed_UnknownPacket_RecentPackets_Empty` | RecentPackets is empty when no prior dispatch |
| `TestMapSession_Feed_UnknownPacket_RawBuffer_IsCopy` | RawBuffer is heap copy, safe to retain |
| `TestRecentRing_ChronologicalOrder` | RecentPackets ordered oldest→newest when < depth packets dispatched |
| `TestRecentRing_WrapEvictsOldest` | Ring wraps correctly; oldest evicted; only most recent depth entries present |
| `TestRecentRing_FrameBytes` | Frame bytes match original; FrameTotal and Truncated correct for small frame |
| `TestRecentRing_FrameIsCopy` | Mutating session buffer after callback does not corrupt retained frame bytes |
| `TestMapPhase_UnknownPacketBeforeReady_Regression` (fsm) | OnReady does not fire; session times out rather than faulting |

---

## Benchmark Results

```
BenchmarkFeed_SmallFixedPacket-14         57298761    17.89 ns/op   0 B/op   0 allocs/op
BenchmarkFeed_VariableLengthPacket-14     75647733    18.07 ns/op   0 B/op   0 allocs/op
BenchmarkFeed_ActorExists_0x09FF-14       69486739    19.05 ns/op   0 B/op   0 allocs/op
BenchmarkEncode_RequestMove-14          1000000000     0.40 ns/op   0 B/op   0 allocs/op
BenchmarkFeed_1000Sessions_Parallel-14   251913816     5.15 ns/op   0 B/op   0 allocs/op
```

Zero allocations on the hot path. The unknown-packet path allocates
(`snapshot()`, `time.Now()`, `RawBuffer` copy) but this is acceptable — it
only fires on truly unknown IDs, which should be rare in a correctly configured
deployment.

---

## Notes

- `ErrUnknownPacket` and `faulted` are kept for genuine stream corruption
  (variable-length embedded length < 4). That case is unrecoverable.
- Session recovery after an unknown packet depends on the server sending a known
  packet ID in the next TCP read. It is best-effort. The real fix is always
  keeping the length table complete for the target packetver.
- `recentMaxFrameBytes = 4096` is a compile-time constant. If a frame is larger,
  `Truncated = true` and `Frame` contains the first 4096 bytes. The ID and
  `FrameTotal` are always accurate.
