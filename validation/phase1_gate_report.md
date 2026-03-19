# Phase 1 Gate Verification Report

**Generated**: 2026-03-18T10:41:06-07:00
**rAthena path**: /home/mikekao/personal/rathena
**PACKETVER**: 20180307 (modern), 20160101 (old)

## 1. Login Server Packets

- [x] **Login/0x0064**: CA_LOGIN packet ID = 0x64
- [x] **Login/0x0069**: AC_ACCEPT_LOGIN (old) packet ID = 0x69 at PACKETVER < 20170315
- [x] **Login/0x0AC4**: AC_ACCEPT_LOGIN (modern) packet ID = 0xAC4 at PACKETVER >= 20170315
- [x] **Login/0x006A**: AC_REFUSE_LOGIN (old) = 0x6a at PACKETVER < 20120000
- [x] **Login/0x083E**: AC_REFUSE_LOGIN (modern) = 0x83e at PACKETVER >= 20120000

## 2. Char Server Packets

- [ ] **Char/0x0065**: CH_MAKE_CHAR base ID 0x65 not found (requires shuffle table lookup)
- [x] **Char/0x082D**: HC_ACCEPT_ENTER2 packet ID = 0x82d
- [x] **Char/0x006B**: HC_ACCEPT_ENTER packet ID = 0x6b
- [x] **Char/0x09A0**: HC_CHARLIST_NOTIFY packet ID = 0x9a0
- [x] **Char/0x099D**: HC_ACK_CHARINFO_PER_PAGE packet ID = 0x99d
- [x] **Char/0x0066**: CH_SELECT_CHAR packet ID = 0x66
- [x] **Char/0x0081**: HC_NOTIFY_ZONESVR = 0x81 (PACKETVER < 20170315)
- [x] **Char/0x0AC5**: HC_NOTIFY_ZONESVR = 0xAC5 (PACKETVER >= 20170315)
- [x] **Char/0x0081-SC_NOTIFY_BAN**: SC_NOTIFY_BAN = 0x81 (collides with HC_NOTIFY_ZONESVR)
- [x] **Char/0x006C**: HC_REFUSE_ENTER packet ID = 0x6c

## 3. Map Server Packets

- [x] **Map/0x0436**: CZ_ENTER2 packet ID = 0x436 (PacketDB)
- [x] **Map/0x0283**: ZC_AID packet ID = 0x283 (PacketDB)
- [x] **Map/0x02EB**: ZC_ACCEPT_ENTER packet ID = 0x2EB at PACKETVER >= 20130710
- [x] **Map/0x007D**: CZ_CLIENTTYPE packet ID = 0x7d (PacketDB)
