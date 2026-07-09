// Package events — hand-maintained (codegen pipeline deprecated for this packet).
package events

// ZcNotifyMapproperty2 is the event emitted for the zc_notify_mapproperty2
// action. It covers the full rAthena map-property packet family cataloged across
// every relevant PACKETVER. The family is two distinct rAthena send paths:
//
//  1. clif_map_property (src/map/clif.cpp:6871-6903) — "property + flags", sent at
//     map entry (clif.cpp:10836-10844), PvP/PK zone changes, and duel start/stop.
//     Type carries the map_property enum; Flags is the bitfield (0x099B only).
//     - PACKETVER <  20121010: 0x0199, 4 bytes. ZC_NOTIFY_MAPPROPERTY. Flags = 0.
//     - PACKETVER >= 20121010: 0x099B, 8 bytes. ZC_MAPPROPERTY_R2. Flags = WBUFL(buf,4).
//
//  2. clif_map_type (src/map/clif.cpp:6907-6914) — separate packet 0x01D6, 4 bytes,
//     sent for battlegrounds (clif.cpp:11071). ZC_NOTIFY_MAPPROPERTY2
//     (packets.hpp:966-969). Type carries the e_map_type enum; Flags = 0.
//
// Note: Type's meaning depends on the source packet (map_property for 0x0199/0x099B,
// e_map_type for 0x01D6). The two enums have overlapping ranges; consumers that
// interpret Type must know which variant arrived. Flags distinguishes the 0x099B
// modern map-property packet from the others (nonzero only for 0x099B).
type ZcNotifyMapproperty2 struct {
	Type  int16
	Flags uint32
}

// MapProperty enumerates the values of the map_property field (clif.hpp:365-373),
// carried by Type for the 0x0199 and 0x099B variants of clif_map_property.
type MapProperty int16

const (
	MapPropertyNothing       MapProperty = 0
	MapPropertyFreePvpZone   MapProperty = 1
	MapPropertyEventPvpZone  MapProperty = 2
	MapPropertyAgitZone      MapProperty = 3
	MapPropertyPkServerZone  MapProperty = 4
	MapPropertyPvpServerZone MapProperty = 5
	MapPropertyDenySkillZone MapProperty = 6
)

// MapType enumerates the values of the e_map_type field (clif.hpp:376-402),
// carried by Type for the 0x01D6 variant (clif_map_type). Gaps (21-24, 26-28) are
// undefined in rAthena and omitted here.
type MapType int16

const (
	MapTypeVillage             MapType = 0
	MapTypeVillageIn           MapType = 1
	MapTypeField               MapType = 2
	MapTypeDungeon             MapType = 3
	MapTypeArena               MapType = 4
	MapTypePenaltyFreePkZone   MapType = 5
	MapTypeNoPenaltyFreePkZone MapType = 6
	MapTypeEventGuildWar       MapType = 7
	MapTypeAgit                MapType = 8
	MapTypeDungeon2            MapType = 9
	MapTypeDungeon3            MapType = 10
	MapTypePkServer            MapType = 11
	MapTypePvpServer           MapType = 12
	MapTypeDenySkill           MapType = 13
	MapTypeTurboTrack          MapType = 14
	MapTypeJail                MapType = 15
	MapTypeMonsterTrack        MapType = 16
	MapTypePoringBattle        MapType = 17
	MapTypeAgitSiegeV15        MapType = 18
	MapTypeBattlefield         MapType = 19
	MapTypePvpTournament       MapType = 20
	MapTypeSiegeLowLevel       MapType = 25
	MapTypeUnused              MapType = 29
)

// MapPropertyFlag bits occupy the Flags bitfield (WBUFL(buf,4), clif.cpp:6888-6898)
// of the 0x099B ZC_MAPPROPERTY_R2 variant. Flags is 0 for the 0x0199 and 0x01D6
// variants which have no bitfield. Bit meanings (clif.cpp:6888-6898):
//
//	bit 0  PARTY              show attack cursor on non-party members (PvP)
//	bit 1  GUILD              show attack cursor on non-guild members (GvG)
//	bit 2  SIEGE              show emblem over characters in GvG (WoE)
//	bit 3  USE_SIMPLE_EFFECT  force /mineffect
//	bit 4  DISABLE_LOCKON     attacks need shift/ns
//	bit 5  COUNT_PK           show PvP counter
//	bit 6  NO_PARTY_FORMATION prevent party create/modify
//	bit 7  BATTLEFIELD        battleground area
//	bit 8  DISABLE_COSTUME    disable costume sprites
//	bit 9  USECART            allow cart inventory
//	bit 10 SUNMOONSTAR_MIRACLE allow Star Gladiator miracle
const (
	MapPropertyFlagParty              uint32 = 1 << 0
	MapPropertyFlagGuild              uint32 = 1 << 1
	MapPropertyFlagSiege              uint32 = 1 << 2
	MapPropertyFlagUseSimpleEffect    uint32 = 1 << 3
	MapPropertyFlagDisableLockon      uint32 = 1 << 4
	MapPropertyFlagCountPk            uint32 = 1 << 5
	MapPropertyFlagNoPartyFormation   uint32 = 1 << 6
	MapPropertyFlagBattlefield        uint32 = 1 << 7
	MapPropertyFlagDisableCostume     uint32 = 1 << 8
	MapPropertyFlagUseCart            uint32 = 1 << 9
	MapPropertyFlagSunmoonstarMiracle uint32 = 1 << 10
)
