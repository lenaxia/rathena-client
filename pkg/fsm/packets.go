package fsm

import (
	"encoding/binary"
)

// buildLoginPacket builds the 0x0064 CA_LOGIN packet.
//
// Source: common/packets.hpp PACKET_CA_LOGIN (verified with GCC at PACKETVER=20180307):
//
//	struct PACKET_CA_LOGIN {
//	    int16  packetType;          // 0x0064
//	    uint32 version;
//	    char   username[24];        // (23+1) null-terminated
//	    char   password[24];        // (23+1) null-terminated
//	    uint8  clienttype;
//	} __attribute__((packed));      // total = 2+4+24+24+1 = 55 bytes
func buildLoginPacket(packetver uint32, username, password string) []byte {
	pkt := make([]byte, 55)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0064)
	binary.LittleEndian.PutUint32(pkt[2:6], packetver)
	copyStr(pkt[6:30], username)  // username[24]
	copyStr(pkt[30:54], password) // password[24]
	pkt[54] = 0x00                // clienttype
	return pkt
}

// buildCharEnterPacket builds the 0x0065 CH_ENTER packet.
//
// Source: char_clif.cpp:820 comment (no struct macro — raw RFIFOSKIP(fd,17)):
//
//	0065 <account id>.L <login id1>.L <login id2>.L <???>.W <sex>.B
//	= int16 + uint32 + uint32 + uint32 + uint16 + uint8 = 17 bytes
func buildCharEnterPacket(accountID, sessionID1, sessionID2 uint32, sex uint8) []byte {
	pkt := make([]byte, 17)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0065)
	binary.LittleEndian.PutUint32(pkt[2:6], accountID)
	binary.LittleEndian.PutUint32(pkt[6:10], sessionID1)
	binary.LittleEndian.PutUint32(pkt[10:14], sessionID2)
	binary.LittleEndian.PutUint16(pkt[14:16], 0) // ??? (clienttype/user level)
	pkt[16] = sex
	return pkt
}

// buildSelectCharPacket builds the 0x0066 CH_SELECT_CHAR packet.
//
// Source: common/packets.hpp PACKET_CH_SELECT_CHAR:
//
//	struct PACKET_CH_SELECT_CHAR {
//	    int16 packetType;   // 0x0066
//	    uint8 slot;
//	} __attribute__((packed));    // total = 3 bytes
func buildSelectCharPacket(slot uint8) []byte {
	pkt := make([]byte, 3)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0066)
	pkt[2] = slot
	return pkt
}

// buildCharlistReq builds the 0x09A1 CH_CHARLIST_REQ packet.
//
// Source: common/packets.hpp PACKET_CH_CHARLIST_REQ:
//
//	struct PACKET_CH_CHARLIST_REQ {
//	    int16 packetType;   // 0x09A1
//	} __attribute__((packed));    // total = 2 bytes
func buildCharlistReq() []byte {
	pkt := make([]byte, 2)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x09A1)
	return pkt
}

// buildMapEnterPacket builds the 0x0436 CZ_ENTER2 packet.
//
// Source: clif.cpp:10641 comment:
//
//	0436 <account id>.L <char id>.L <auth code>.L <client time>.L <gender>.B (CZ_ENTER2)
//	= int16 + uint32 + uint32 + uint32 + uint32 + uint8 = 19 bytes
//
// Note: the packet ID field is part of the byte slice so that the caller can
// apply obfuscation via MapSession.Encode before writing to the socket.
// But the FSM builds this as a plain slice; the caller applies Encode separately
// to the ID bytes before writing.
func buildMapEnterPacket(accountID, charID, sessionID1 uint32, sex uint8) []byte {
	pkt := make([]byte, 19)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x0436)
	binary.LittleEndian.PutUint32(pkt[2:6], accountID)
	binary.LittleEndian.PutUint32(pkt[6:10], charID)
	binary.LittleEndian.PutUint32(pkt[10:14], sessionID1)
	binary.LittleEndian.PutUint32(pkt[14:18], 0) // client tick — always 0 at connect
	pkt[18] = sex
	return pkt
}

// buildMapLoadedPacket builds the 0x007D CZ_NOTIFY_ACTORINIT packet.
//
// Source: clif.cpp:10742:
//
//	"Notification from the client, that it has finished map loading"
//	0x007D — no fields after packetType = 2 bytes total
func buildMapLoadedPacket() []byte {
	pkt := make([]byte, 2)
	binary.LittleEndian.PutUint16(pkt[0:2], 0x007D)
	return pkt
}

// buildTickSyncPacket builds the 0x007E (CZ_REQUEST_TIME) or 0x0360
// (CZ_REQUEST_TIME2) tick-sync packet.
//
// Source: clif.cpp:11196-11197:
//
//	007e <client tick>.L (CZ_REQUEST_TIME)
//	0360 <client tick>.L (CZ_REQUEST_TIME2)
//	= int16 + uint32 = 6 bytes
//
// The returned tickID is the raw (unobfuscated) packet ID; the caller must
// apply MapSession.Encode(&tickID) before writing.
// The packetID is embedded in pkt[0:2] and also returned separately so the
// caller can pass it to MapSession.Encode.
func buildTickSyncPacket(packetver uint32) (tickID uint16, pkt []byte) {
	id := uint16(0x007E)
	if packetver >= 20080102 {
		id = 0x0360
	}
	pkt = make([]byte, 6)
	binary.LittleEndian.PutUint16(pkt[0:2], id)
	binary.LittleEndian.PutUint32(pkt[2:6], 0) // client tick — 0 at connect
	return id, pkt
}

// copyStr copies s into dst as a null-terminated C string, truncating if needed.
// It always zero-fills the remainder of dst.
func copyStr(dst []byte, s string) {
	n := copy(dst, s)
	for i := n; i < len(dst); i++ {
		dst[i] = 0
	}
}
