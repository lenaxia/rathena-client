#!/usr/bin/env bash
# preprocess_check.sh — run GCC -E on all three rAthena packet headers
#
# Usage: ./validation/preprocess_check.sh [PACKETVER]
#   PACKETVER defaults to 20180307 (last version with obfuscation enabled)
#
# Output: validation/output/packets_struct_PACKETVER.h
#         validation/output/packets_PACKETVER.h
#         validation/output/common_packets_PACKETVER.h
#
# Exit code 0 = all three headers preprocess successfully and produce struct output.
# Exit code 1 = one or more failed.

set -euo pipefail

PACKETVER="${1:-20180307}"
RATHENA="${RATHENA_ROOT:-$HOME/personal/rathena}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
STUBS="$REPO/validation/stubs"
OUT="$REPO/validation/output"

mkdir -p "$OUT"

COMMON_FLAGS=(
    -E -P
    "-DPACKETVER=$PACKETVER"
    "-DPACKETVER_MAIN_NUM=$PACKETVER"
    "-I$RATHENA/src"
    "-I$RATHENA/src/map"
    "-I$RATHENA/src/common"
)

OK=0

echo "=== preprocess_check.sh PACKETVER=$PACKETVER ==="

# --- packets_struct.hpp (no stubs needed) ---
echo -n "packets_struct.hpp ... "
if g++ "${COMMON_FLAGS[@]}" \
        "$RATHENA/src/map/packets_struct.hpp" \
        > "$OUT/packets_struct_${PACKETVER}.h" 2>/dev/null; then
    STRUCT_COUNT=$(grep -c "^struct " "$OUT/packets_struct_${PACKETVER}.h" || true)
    echo "OK ($STRUCT_COUNT structs)"
else
    echo "FAILED"
    g++ "${COMMON_FLAGS[@]}" "$RATHENA/src/map/packets_struct.hpp" 2>&1 | grep "error:\|fatal:" | head -5
    OK=1
fi

# --- packets.hpp (needs packets_hpp_stub.h) ---
echo -n "packets.hpp ... "
if g++ "${COMMON_FLAGS[@]}" \
        "-include$STUBS/packets_hpp_stub.h" \
        "$RATHENA/src/map/packets.hpp" \
        > "$OUT/packets_${PACKETVER}.h" 2>/dev/null; then
    STRUCT_COUNT=$(grep -c "^struct " "$OUT/packets_${PACKETVER}.h" || true)
    echo "OK ($STRUCT_COUNT structs)"
else
    echo "FAILED"
    g++ "${COMMON_FLAGS[@]}" "-include$STUBS/packets_hpp_stub.h" \
        "$RATHENA/src/map/packets.hpp" 2>&1 | grep "error:\|fatal:" | head -5
    OK=1
fi

# --- common/packets.hpp (needs common_hpp_stub.h) ---
echo -n "common/packets.hpp ... "
if g++ -E -P \
        "-DPACKETVER=$PACKETVER" \
        "-DPACKETVER_MAIN_NUM=$PACKETVER" \
        "-I$RATHENA/src" \
        "-I$RATHENA/src/common" \
        "-include$STUBS/common_hpp_stub.h" \
        "$RATHENA/src/common/packets.hpp" \
        > "$OUT/common_packets_${PACKETVER}.h" 2>/dev/null; then
    STRUCT_COUNT=$(grep -c "^struct " "$OUT/common_packets_${PACKETVER}.h" || true)
    echo "OK ($STRUCT_COUNT structs)"
else
    echo "FAILED"
    g++ -E -P "-DPACKETVER=$PACKETVER" "-DPACKETVER_MAIN_NUM=$PACKETVER" \
        "-I$RATHENA/src" "-I$RATHENA/src/common" \
        "-include$STUBS/common_hpp_stub.h" \
        "$RATHENA/src/common/packets.hpp" 2>&1 | grep "error:\|fatal:" | head -5
    OK=1
fi

# --- clif_obfuscation.hpp (needs -DPACKET_OBFUSCATION) ---
echo -n "clif_obfuscation.hpp ... "
if g++ -E -P \
        "-DPACKETVER=$PACKETVER" \
        -DPACKET_OBFUSCATION \
        "-I$RATHENA/src" \
        "$RATHENA/src/map/clif_obfuscation.hpp" \
        > "$OUT/clif_obfuscation_${PACKETVER}.h" 2>/dev/null; then
    KEY_COUNT=$(grep -c "clif_cryptKey" "$OUT/clif_obfuscation_${PACKETVER}.h" || true)
    echo "OK ($KEY_COUNT key definitions)"
else
    echo "FAILED (may be expected for PACKETVER not in obfuscation table)"
    g++ -E -P "-DPACKETVER=$PACKETVER" -DPACKET_OBFUSCATION \
        "-I$RATHENA/src" "$RATHENA/src/map/clif_obfuscation.hpp" 2>&1 | head -3
fi

echo ""
if [ $OK -eq 0 ]; then
    echo "All headers preprocessed successfully. Output in: $OUT/"
else
    echo "One or more headers FAILED. Fix stubs before proceeding."
fi

exit $OK
