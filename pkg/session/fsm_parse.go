package session

import (
	"encoding/binary"
	"fmt"
)

// parseLoginAccept parses the 0x0069 (pv < 20170315) or 0x0AC4 (pv >= 20170315)
// login accept packet and returns the char server list plus auth tokens.
//
// Source: common/packets.hpp PACKET_AC_ACCEPT_LOGIN (verified with GCC):
//
// Pre-20170315 (0x0069):
//
//	struct PACKET_AC_ACCEPT_LOGIN {
//	    int16  packetType;          // 2
//	    int16  packetLength;        // 2
//	    uint32 login_id1;           // 4
//	    uint32 AID;                 // 4
//	    uint32 login_id2;           // 4
//	    uint32 last_ip;             // 4
//	    char   last_login[26];      // 26
//	    uint8  sex;                 // 1   → offset 47
//	    PACKET_AC_ACCEPT_LOGIN_sub char_servers[];
//	} // header = 47 bytes
//
// Sub-entry pre-20170315 (PACKET_AC_ACCEPT_LOGIN_sub, 32 bytes):
//
//	uint32 ip; uint16 port; char name[20]; uint16 users; uint16 type; uint16 new_;
//	= 4+2+20+2+2+2 = 32 bytes
//
// Post-20170315 (0x0AC4): same header + char token[17] = 47+17=64 bytes header.
// Sub-entry post-20170315: adds uint8 unknown[128] = 32+128 = 160 bytes.
func parseLoginAccept(data []byte, packetver uint32) (
	servers []CharServerInfo, accountID, sessionID1, sessionID2 uint32, sex uint8, err error,
) {
	const headerPre = 47
	const headerPost = 64 // 47 + 17 (token)
	const subPre = 32
	const subPost = 160

	hdrSize := headerPre
	subSize := subPre
	if packetver >= 20170315 {
		hdrSize = headerPost
		subSize = subPost
	}

	if len(data) < hdrSize {
		err = fmt.Errorf("fsm: login accept packet too short (%d < %d)", len(data), hdrSize)
		return
	}

	// Fields (common between old and new):
	// [0:2]   packetType
	// [2:4]   packetLength
	// [4:8]   login_id1
	// [8:12]  AID
	// [12:16] login_id2
	// [16:20] last_ip
	// [20:46] last_login[26]
	// [46]    sex
	sessionID1 = binary.LittleEndian.Uint32(data[4:8])
	accountID = binary.LittleEndian.Uint32(data[8:12])
	sessionID2 = binary.LittleEndian.Uint32(data[12:16])
	sex = data[46]

	payload := data[hdrSize:]
	for len(payload) >= subSize {
		// rAthena writes: char_server.ip = htonl(...) — network byte order (big-endian).
		// Source: loginclif.cpp:137 "char_server.ip = htonl(...)"
		ip := binary.BigEndian.Uint32(payload[0:4])
		port := binary.LittleEndian.Uint16(payload[4:6])
		name := fsmCStr(payload[6:26])
		servers = append(servers, CharServerInfo{
			IP:   ip,
			Port: port,
			Name: name,
		})
		payload = payload[subSize:]
	}
	return
}

// fsmCStr converts a null-terminated C string byte slice to a Go string.
func fsmCStr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
