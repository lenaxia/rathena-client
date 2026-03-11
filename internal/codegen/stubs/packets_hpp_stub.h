// packets_hpp_stub.h — stub headers required to preprocess src/map/packets.hpp
//
// packets.hpp includes map.hpp, which includes script.hpp, which includes
// ryml_std.hpp (a third-party YAML library not present in this repo).
// This stub provides exactly what packets.hpp needs from that chain,
// then defines include guards to prevent the problematic files from loading.
//
// What packets.hpp actually uses from map.hpp (verified by inspection):
//   MESSAGE_SIZE, TALKBOX_MESSAGE_SIZE   — from common/mmo.hpp
//   MAX_ITEM_RDM_OPT                     — from common/mmo.hpp
//   MAX_INVENTORY, MAX_CART, MAX_STORAGE — from common/mmo.hpp
//
// Strategy: define the include guards for map.hpp, script.hpp, and all their
// problematic transitive includes, then define the constants directly.

// Block map.hpp and its entire include chain
#define MAP_HPP
#define SCRIPT_HPP
#define NAVI_HPP
#define PATH_HPP
#define DATABASE_HPP
#define YAML_HPP

// Block the ryml headers
#define RYML_SINGLE_HPP_DEFINE_FUNCTIONS
#define _RYML_HPP_
#define RYML_STD_HPP_
#define C4_RYML_HPP_

// Block common headers that pull in libconfig/mysql
#define SHOWMSG_HPP
#define SQL_HPP
#define MSG_CONF_HPP

// Block other common headers with external deps
#define DB_HPP
#define TIMER_HPP
#define CORE_HPP
#define SOCKET_HPP
#define NULLPO_HPP
#define MAPINDEX_HPP
#define UTILITIES_HPP

// Now define the constants that packets.hpp actually needs
// Source: common/mmo.hpp (values verified from rAthena source)
#define MESSAGE_SIZE (79 + 1)
#define TALKBOX_MESSAGE_SIZE (79 + 1)

#define INVENTORY_BASE_SIZE 100
// INVENTORY_EXPANSION_SIZE is hardcoded to 0. rAthena sets this to 100 via
// battle_config.inventory_expansion_size at runtime, but packet struct sizes
// that embed it (e.g. item list packets) use its compile-time value. Setting
// it to 0 matches the default disabled state and produces struct sizes that
// agree with the recvpackets.txt lengths used by the framing engine.
#define INVENTORY_EXPANSION_SIZE 0
#define MAX_INVENTORY 100
#define MAX_CART 100
#define MAX_STORAGE 600
#define MAX_ITEM_RDM_OPT 5

#define MAP_NAME_LENGTH (11 + 1)
#define MAP_NAME_LENGTH_EXT (MAP_NAME_LENGTH + 4)
#define NAME_LENGTH (23 + 1)

// Minimal cbasetypes that cbasetypes.hpp would normally provide
// (only what packets.hpp actually uses)
#include <stdint.h>
typedef uint8_t  uint8;
typedef uint16_t uint16;
typedef uint32_t uint32;
typedef uint64_t uint64;
typedef int8_t   int8;
typedef int16_t  int16;
typedef int32_t  int32;
typedef int64_t  int64;
typedef unsigned char uchar;
typedef unsigned short ushort;
typedef unsigned int uint;
