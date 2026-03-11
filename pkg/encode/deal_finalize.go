// Hand-written: PACKET_CZ_CONCLUDE_EXCHANGE_ITEM (0x00EB) has no rAthena struct
// definition — it is a 2-byte header-only packet parsed via a raw clif_packetdb
// entry (parseable_packet(0x00eb,2,clif_parse_TradeOk,0)). There are no fields
// beyond the packet type, so no struct is needed and no struct-derived codegen
// is possible.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeDealFinalize encodes a 0x00EB (CZ_CONCLUDE_EXCHANGE_ITEM) packet.
// The packet carries no payload beyond the 2-byte packet type — it is the
// client's "OK" confirmation when finalising a trade.
func EncodeDealFinalize(_ send.DealFinalize, _ uint32) [2]byte {
	return [2]byte{0xeb, 0x00}
}
