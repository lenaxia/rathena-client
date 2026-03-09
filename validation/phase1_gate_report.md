# Phase 1 Gate Verification Report

**Generated**: 2026-03-09T15:00:55-07:00
**rAthena path**: /home/mikekao/personal/rathena
**PACKETVER**: 20180307 (modern), 20160101 (old)

## 1. Login Server Packets

- [x] **Layout/packet_idle_unit/pre-20080102**: packet_idle_unit 56B packed layout at 20080101
- [x] **Layout/packet_idle_unit/20080102**: packet_idle_unit 60B packed layout at 20080102
- [x] **Layout/packet_idle_unit/20091103**: packet_idle_unit 63B packed layout at 20091103
- [x] **Login/0x0064**: CA_LOGIN packet ID = 0x64
- [x] **Login/0x0069**: AC_ACCEPT_LOGIN (old) packet ID = 0x69 at PACKETVER < 20170315
- [x] **Login/0x0AC4**: AC_ACCEPT_LOGIN (modern) packet ID = 0xAC4 at PACKETVER >= 20170315
- [x] **Layout/packet_idle_unit/20101124**: packet_idle_unit 65B packed layout at 20101124
- [x] **Layout/packet_idle_unit/20120221**: packet_idle_unit 74B packed layout at 20120221
- [x] **Login/0x006A**: AC_REFUSE_LOGIN (old) = 0x6a at PACKETVER < 20120000
- [x] **Layout/packet_idle_unit/20131223**: packet_idle_unit 102B packed layout at 20131223
- [x] **Login/0x083E**: AC_REFUSE_LOGIN (modern) = 0x83e at PACKETVER >= 20120000

## 2. Char Server Packets

- [x] **Layout/packet_idle_unit/20150513**: packet_idle_unit 104B packed layout at 20150513
- [ ] **Char/0x0065**: CH_MAKE_CHAR base ID 0x65 not found (requires shuffle table lookup)
- [x] **Layout/packet_idle_unit/20181121**: packet_idle_unit 108B packed layout at 20181121
- [x] **Char/0x082D**: HC_ACCEPT_ENTER2 packet ID = 0x82d
- [x] **Char/0x006B**: HC_ACCEPT_ENTER packet ID = 0x6b
- [x] **Char/0x09A0**: HC_CHARLIST_NOTIFY packet ID = 0x9a0
- [x] **Char/0x099D**: HC_ACK_CHARINFO_PER_PAGE packet ID = 0x99d
- [x] **Char/0x0066**: CH_SELECT_CHAR packet ID = 0x66
- [x] **Char/0x0081**: HC_NOTIFY_ZONESVR = 0x81 (PACKETVER < 20170315)
- [x] **Layout/packet_spawn_unit/pre-20080102**: packet_spawn_unit 55B packed layout at 20080101
- [x] **Char/0x0AC5**: HC_NOTIFY_ZONESVR = 0xAC5 (PACKETVER >= 20170315)
- [x] **Char/0x0081-SC_NOTIFY_BAN**: SC_NOTIFY_BAN = 0x81 (collides with HC_NOTIFY_ZONESVR)
- [x] **Char/0x006C**: HC_REFUSE_ENTER packet ID = 0x6c

## 3. Map Server Packets

- [x] **Layout/packet_spawn_unit/20080102**: packet_spawn_unit 59B packed layout at 20080102
- [x] **Map/0x0436**: CZ_ENTER2 packet ID = 0x436 (PacketDB)
- [x] **Map/0x0283**: ZC_AID packet ID = 0x283 (PacketDB)
- [x] **Map/0x02EB**: ZC_ACCEPT_ENTER packet ID = 0x2EB at PACKETVER >= 20130710
- [x] **Map/0x007D**: CZ_CLIENTTYPE packet ID = 0x7d (PacketDB)
- [x] **Layout/packet_spawn_unit/20091103**: packet_spawn_unit 62B packed layout at 20091103
- [x] **Map/0x007E**: CZ_REQUEST_TIME packet ID = 0x7e (PacketDB)
- [x] **Map/0x0360**: CZ_REQUEST_TIME2 packet ID = 0x360 (PacketDB)
- [x] **Map/0x0074**: ZC_REFUSE_ENTER packet ID = 0x74 (HEADER)

## 4. Actor Visibility Packets (S→C)

- [x] **Actor/0x007B**: ZC_NOTIFY_MOVE = 0x7B (PacketDB)
- [x] **Actor/0x0080**: ZC_NOTIFY_VANISH = 0x80 (HEADER)
- [x] **Stat/0x00B0**: ZC_PAR_CHANGE = 0xB0 (HEADER)
- [x] **Layout/packet_spawn_unit/20101124**: packet_spawn_unit 64B packed layout at 20101124
- [x] **Stat/0x00B1**: ZC_LONGPAR_CHANGE = 0xB1 (HEADER)

## 5. Send Path Packets (C→S)

- [x] **Send/0x0085**: CZ_REQUEST_MOVE = 0x85 (PacketDB, base ID)
- [x] **Ping/0x0B1D**: ZC_PING_LIVE = 0x0B1D (HEADER, PACKETVER >= 20190220)
- [x] **Ping/0x0B1C**: CZ_PING_LIVE = 0x0B1C (HEADER, PACKETVER >= 20190220)

## 6. Struct Layout Verification

Field layouts verified at every PACKETVER breakpoint for each struct.
All sizes are __attribute__((packed)) — zero alignment padding.
PosDir/MoveData fields are noted as WBUFPOS/WBUFPOS2 packed bit encodings.

- [x] **Layout/packet_spawn_unit/20120221**: packet_spawn_unit 73B packed layout at 20120221
- [x] **Layout/packet_idle_unit/pre-20080102**: packet_idle_unit 56B packed layout at 20080101
- [x] **Layout/packet_spawn_unit/20131223**: packet_spawn_unit 101B packed layout at 20131223
- [x] **Layout/packet_idle_unit/20080102**: packet_idle_unit 60B packed layout at 20080102
- [x] **Layout/packet_spawn_unit/20150513**: packet_spawn_unit 103B packed layout at 20150513
- [x] **Layout/packet_idle_unit/20091103**: packet_idle_unit 63B packed layout at 20091103
- [x] **Layout/packet_spawn_unit/20181121**: packet_spawn_unit 107B packed layout at 20181121
- [x] **Layout/packet_idle_unit/20101124**: packet_idle_unit 65B packed layout at 20101124
- [x] **Layout/packet_unit_walking/pre-20071106**: packet_unit_walking 64B packed layout at 20071105
- [x] **Layout/packet_idle_unit/20120221**: packet_idle_unit 74B packed layout at 20120221
- [x] **Layout/packet_unit_walking/20071106**: packet_unit_walking 65B packed layout at 20071106
- [x] **Layout/packet_idle_unit/20131223**: packet_idle_unit 102B packed layout at 20131223
- [x] **Layout/packet_unit_walking/20080102**: packet_unit_walking 67B packed layout at 20080102
- [x] **Layout/packet_idle_unit/20150513**: packet_idle_unit 104B packed layout at 20150513
- [x] **Layout/packet_unit_walking/20091103**: packet_unit_walking 69B packed layout at 20091103
- [x] **Layout/packet_unit_walking/20101124**: packet_unit_walking 71B packed layout at 20101124
- [x] **Layout/packet_idle_unit/20181121**: packet_idle_unit 108B packed layout at 20181121
- [x] **Layout/packet_unit_walking/20120221**: packet_unit_walking 80B packed layout at 20120221
- [x] **Layout/packet_spawn_unit/pre-20080102**: packet_spawn_unit 55B packed layout at 20080101
- [x] **Layout/packet_spawn_unit/20080102**: packet_spawn_unit 59B packed layout at 20080102
- [x] **Layout/packet_unit_walking/20131223**: packet_unit_walking 108B packed layout at 20131223
- [x] **Layout/packet_spawn_unit/20091103**: packet_spawn_unit 62B packed layout at 20091103
- [x] **Layout/packet_unit_walking/20150513**: packet_unit_walking 110B packed layout at 20150513
- [x] **Layout/packet_spawn_unit/20101124**: packet_spawn_unit 64B packed layout at 20101124
- [x] **Layout/packet_unit_walking/20181121**: packet_unit_walking 114B packed layout at 20181121
- [x] **Layout/packet_spawn_unit/20120221**: packet_spawn_unit 73B packed layout at 20120221
- [x] **Layout/packet_authok/pre-20080102**: packet_authok 11B packed layout at 20080101
- [x] **Layout/packet_authok/20080102**: packet_authok 13B packed layout at 20080102
- [x] **Layout/packet_spawn_unit/20131223**: packet_spawn_unit 101B packed layout at 20131223
- [x] **Layout/packet_authok/20141022**: packet_authok 14B packed layout at 20141022
- [x] **Layout/packet_spawn_unit/20150513**: packet_spawn_unit 103B packed layout at 20150513
- [x] **Layout/packet_authok/20160330**: packet_authok 13B packed layout at 20160330
- [x] **Layout/packet_spawn_unit/20181121**: packet_spawn_unit 107B packed layout at 20181121
- [x] **Layout/packet_idle_unit2/pre-20071106**: packet_idle_unit2 54B packed layout at 20071105
- [x] **Layout/packet_unit_walking/pre-20071106**: packet_unit_walking 64B packed layout at 20071105
- [x] **Layout/packet_idle_unit2/20071106**: packet_idle_unit2 55B packed layout at 20071106
- [x] **Layout/packet_idle_unit2/20091102**: packet_idle_unit2 55B packed layout at 20091102
- [x] **Layout/packet_unit_walking/20071106**: packet_unit_walking 65B packed layout at 20071106
- [x] **Layout/packet_idle_unit2/20091103-tombstone**: packet_idle_unit2 UNAVAILABLE_STRUCT tombstone at 20091103
- [x] **Layout/packet_spawn_unit2/20091102**: packet_spawn_unit2 42B packed layout at 20091102
- [x] **Layout/packet_unit_walking/20080102**: packet_unit_walking 67B packed layout at 20080102
- [x] **Layout/packet_spawn_unit2/20091103-tombstone**: packet_spawn_unit2 UNAVAILABLE_STRUCT tombstone at 20091103
- [x] **Layout/packet_unit_walking/20091103**: packet_unit_walking 69B packed layout at 20091103
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/pre-20080102**: PACKET_ZC_ACCEPT_ENTER 11B packed layout at 20070101
- [x] **Layout/packet_unit_walking/20101124**: packet_unit_walking 71B packed layout at 20101124
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/20080102**: PACKET_ZC_ACCEPT_ENTER 13B packed layout at 20080102
- [x] **Layout/packet_unit_walking/20120221**: packet_unit_walking 80B packed layout at 20120221
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/20141022**: PACKET_ZC_ACCEPT_ENTER 14B packed layout at 20141022
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/20160330**: PACKET_ZC_ACCEPT_ENTER 13B packed layout at 20160330
- [x] **Layout/packet_unit_walking/20131223**: packet_unit_walking 108B packed layout at 20131223
- [x] **Layout/PACKET_AC_ACCEPT_LOGIN_sub/pre-20170315**: PACKET_AC_ACCEPT_LOGIN_sub 32B packed layout at 20170314
- [x] **Layout/packet_unit_walking/20150513**: packet_unit_walking 110B packed layout at 20150513
- [x] **Layout/PACKET_AC_ACCEPT_LOGIN_sub/20170315**: PACKET_AC_ACCEPT_LOGIN_sub 160B packed layout at 20170315
- [x] **Layout/packet_unit_walking/20181121**: packet_unit_walking 114B packed layout at 20181121
- [x] **Layout/CHARACTER_INFO/pre-20180307**: CHARACTER_INFO 147B packed layout at 20170315
- [x] **Layout/packet_authok/pre-20080102**: packet_authok 11B packed layout at 20080101
- [x] **Layout/packet_authok/20080102**: packet_authok 13B packed layout at 20080102
- [x] **Layout/CHARACTER_INFO/20180307**: CHARACTER_INFO 155B packed layout at 20180307
- [x] **Layout/packet_authok/20141022**: packet_authok 14B packed layout at 20141022
- [x] **Layout/PACKET_HC_NOTIFY_ZONESVR/pre-20170315**: PACKET_HC_NOTIFY_ZONESVR 28B packed layout at 20170314
- [x] **Layout/packet_authok/20160330**: packet_authok 13B packed layout at 20160330
- [x] **Layout/PACKET_HC_NOTIFY_ZONESVR/20170315**: PACKET_HC_NOTIFY_ZONESVR 156B packed layout at 20170315

## 7. Algorithm Verification

- [x] **Algo/WBUFPOS**: WBUFPOS/RBUFPOS found in clif.cpp
- [x] **Algo/WBUFPOS2**: WBUFPOS2/RBUFPOS2 found in clif.cpp
- [x] **Algo/Direction**: DIR_NORTH=0 found in path.hpp

---

## Summary

**PASS**: 76
**FAIL**: 1

❌ Some verifications failed
- [x] **Layout/packet_idle_unit2/pre-20071106**: packet_idle_unit2 54B packed layout at 20071105
- [x] **Layout/packet_idle_unit2/20071106**: packet_idle_unit2 55B packed layout at 20071106
- [x] **Layout/packet_idle_unit2/20091102**: packet_idle_unit2 55B packed layout at 20091102
- [x] **Layout/packet_idle_unit2/20091103-tombstone**: packet_idle_unit2 UNAVAILABLE_STRUCT tombstone at 20091103
- [x] **Layout/packet_spawn_unit2/20091102**: packet_spawn_unit2 42B packed layout at 20091102
- [x] **Layout/packet_spawn_unit2/20091103-tombstone**: packet_spawn_unit2 UNAVAILABLE_STRUCT tombstone at 20091103
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/pre-20080102**: PACKET_ZC_ACCEPT_ENTER 11B packed layout at 20070101
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/20080102**: PACKET_ZC_ACCEPT_ENTER 13B packed layout at 20080102
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/20141022**: PACKET_ZC_ACCEPT_ENTER 14B packed layout at 20141022
- [x] **Layout/PACKET_ZC_ACCEPT_ENTER/20160330**: PACKET_ZC_ACCEPT_ENTER 13B packed layout at 20160330
- [x] **Layout/PACKET_AC_ACCEPT_LOGIN_sub/pre-20170315**: PACKET_AC_ACCEPT_LOGIN_sub 32B packed layout at 20170314
- [x] **Layout/PACKET_AC_ACCEPT_LOGIN_sub/20170315**: PACKET_AC_ACCEPT_LOGIN_sub 160B packed layout at 20170315
- [x] **Layout/CHARACTER_INFO/pre-20180307**: CHARACTER_INFO 147B packed layout at 20170315
- [x] **Layout/CHARACTER_INFO/20180307**: CHARACTER_INFO 155B packed layout at 20180307
- [x] **Layout/PACKET_HC_NOTIFY_ZONESVR/pre-20170315**: PACKET_HC_NOTIFY_ZONESVR 28B packed layout at 20170314
- [x] **Layout/PACKET_HC_NOTIFY_ZONESVR/20170315**: PACKET_HC_NOTIFY_ZONESVR 156B packed layout at 20170315

## 7. Algorithm Verification

- [x] **Algo/WBUFPOS**: WBUFPOS/RBUFPOS found in clif.cpp
- [x] **Algo/WBUFPOS2**: WBUFPOS2/RBUFPOS2 found in clif.cpp
- [x] **Algo/Direction**: DIR_NORTH=0 found in path.hpp

---

## Summary

**PASS**: 76
**FAIL**: 1

❌ Some verifications failed
