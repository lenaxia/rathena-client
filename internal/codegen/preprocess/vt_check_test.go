package preprocess

import (
    "fmt"
    "testing"
)

func TestCheckSendStructs(t *testing.T) {
    cfg := Config{RathenaRoot: "/home/mikekao/personal/rathena"}
    pv := uint32(20200401)
    
    preprocessed, err := Preprocess(cfg, SourcePacketsStruct, pv)
    if err != nil {
        t.Fatal(err)
    }
    db, err := ExtractStructs(preprocessed, pv)
    if err != nil {
        t.Fatal(err)
    }
    
    targets := []string{
        "PACKET_CZ_NOTIFY_ACTORINIT",
        "PACKET_CZ_ITEM_PICKUP",
        "PACKET_CZ_USE_ITEM",
        "PACKET_CZ_REQ_WEAR_EQUIP",
        "PACKET_CZ_REQ_TAKEOFF_EQUIP",
        "PACKET_CZ_CHOOSE_MENU",
        "PACKET_CZ_CLOSE_DIALOG",
        "PACKET_CZ_REQ_NEXT_SCRIPT",
        "PACKET_CZ_INPUT_EDITDLG",
        "PACKET_CZ_INPUT_EDITDLGSTR",
        "PACKET_CZ_MOVE_ITEM_FROM_BODY_TO_STORE",
        "PACKET_CZ_MOVE_ITEM_FROM_STORE_TO_BODY",
        "PACKET_CZ_USE_SKILL_TOGROUND",
        "PACKET_CZ_REQUEST_ACT",
        "PACKET_CZ_REQUEST_TIME",
        "PACKET_CZ_ITEM_THROW",
        "PACKET_CZ_CHANGE_DIRECTION",
        "PACKET_CZ_REQ_EMOTION",
        "PACKET_CZ_USE_SKILL",
        "PACKET_CZ_USE_SKILL_TOGROUND2",
    }
    
    for _, name := range targets {
        if layout, ok := db[name]; ok && layout != nil && layout.Available {
            fmt.Printf("FOUND    %s: %d bytes, %d fields\n", name, layout.TotalSize, len(layout.Fields))
        } else {
            fmt.Printf("MISSING  %s\n", name)
        }
    }
}
