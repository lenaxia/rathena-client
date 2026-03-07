// common_hpp_stub.h — stub headers for src/common/packets.hpp
//
// common/packets.hpp includes:
//   common/cbasetypes.hpp  — safe, no external deps
//   common/mmo.hpp         — needs config/core.hpp which needs libconfig.h
//   common/showmsg.hpp     — needs libconfig.h directly
//   common/socket.hpp      — has external deps
//   common/utilities.hpp   — needs ryml
//
// Strategy: block the problematic headers; provide libconfig types so
// config/core.hpp parses without errors; let cbasetypes.hpp and mmo.hpp
// load normally for their constants and struct definitions.

// Block headers with external dependencies
#define SHOWMSG_HPP
#define SQL_HPP
#define UTILITIES_HPP
#define SOCKET_HPP
#define CONFIG_CORE_HPP

// Provide minimal libconfig types so any remaining references compile
#define LIBCONFIG_H
typedef int config_t;
typedef int config_setting_t;
typedef int config_list_t;
