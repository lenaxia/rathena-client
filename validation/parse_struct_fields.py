#!/usr/bin/env python3
"""
parse_struct_fields.py — Parse a preprocessed rAthena struct body into a field table.

Input: path to a file containing struct body lines (everything between "{" and "}",
       with all PACKETVER conditionals already resolved by g++ -E -P).

Output (tab-separated to stdout):
  field  NAME  TYPE  OFFSET  SIZE  NOTE
  total  BYTES

Exit codes:
  0 — success
  1 — UNAVAILABLE_STRUCT sentinel found (struct is tombstoned)
  2 — parse error
"""

import sys
import re

# Byte sizes for __attribute__((packed)) rAthena types.
# Zero means unknown / skip (e.g. nested struct declarations handled separately).
SIZES = {
    "int8": 1,
    "uint8": 1,
    "char": 1,
    "int16": 2,
    "uint16": 2,
    "int32": 4,
    "uint32": 4,
    "float": 4,
    "int64": 8,
    "uint64": 8,
    "double": 8,
}

# Fields that carry special packed bit encoding (noted in output, not size-adjusted).
SPECIAL_NOTE = {
    "PosDir": "packing=WBUFPOS",
    "MoveData": "packing=WBUFPOS2",
    "posDir": "packing=WBUFPOS",
    "moveData": "packing=WBUFPOS2",
}


def main():
    if len(sys.argv) < 2:
        print("Usage: parse_struct_fields.py <body_file>", file=sys.stderr)
        sys.exit(2)

    body_file = sys.argv[1]
    try:
        with open(body_file) as f:
            text = f.read()
    except OSError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(2)

    if "UNAVAILABLE_STRUCT" in text:
        sys.exit(1)

    lines = text.strip().split("\n")
    offset = 0

    for raw in lines:
        line = raw.strip().rstrip(";")
        if not line:
            continue

        # Array field: TYPE NAME[EXPR]
        # EXPR may be a C constant expression like (23 + 1) or plain integer.
        m = re.match(r"^(\w+)\s+(\w+)\[([^\]]+)\]$", line)
        if m:
            typ, name, expr = m.group(1), m.group(2), m.group(3)
            try:
                # eval is safe here — input is GCC preprocessor output, all macros resolved
                count = int(eval(expr))  # noqa: S307
            except Exception:
                count = 0
            base_sz = SIZES.get(typ, 0)
            sz = base_sz * count
            note = SPECIAL_NOTE.get(name, "")
            print(f"field\t{name}\t{typ}[{count}]\t{offset}\t{sz}\t{note}")
            offset += sz
            continue

        # Scalar field: TYPE NAME
        m = re.match(r"^(\w+)\s+(\w+)$", line)
        if m:
            typ, name = m.group(1), m.group(2)
            sz = SIZES.get(typ, 0)
            note = SPECIAL_NOTE.get(name, "")
            print(f"field\t{name}\t{typ}\t{offset}\t{sz}\t{note}")
            offset += sz
            continue

        # Skip lines we cannot parse (nested struct/union decls, comments, etc.)

    print(f"total\t{offset}")


if __name__ == "__main__":
    main()
