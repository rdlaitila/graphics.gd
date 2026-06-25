package product

import "encoding/json"

// PlayReport is the generic envelope every example's play-bot writes
// to $GDNEXT_PLAY_REPORT and `gdnext ci play-cell` reads back.
// GameData and HudData stay opaque so example-specific fields can
// evolve without forcing a driver update; the driver only gates on
// Success.
type PlayReport struct {
	Success  bool            `json:"success"`
	GameData json.RawMessage `json:"game_data,omitempty"`
	HudData  json.RawMessage `json:"hud_data,omitempty"`
}

// PlayHUDColumn is one column in the bottom-spanning provenance table
// the example overlays when driven by `gdnext ci play-cell`. The
// driver writes a []PlayHUDColumn to $GDNEXT_PLAY_HUD as JSON; the
// example mirrors it back into PlayReport.HudData.
type PlayHUDColumn struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Env var names exchanged between `gdnext ci play-cell` and an
// example's play-bot. Centralised so both sides reference the same
// strings.
const (
	EnvPlay           = "GDNEXT_PLAY"
	EnvPlayReport     = "GDNEXT_PLAY_REPORT"
	EnvPlayScreenshot = "GDNEXT_PLAY_SCREENSHOT"
	EnvPlayHUD        = "GDNEXT_PLAY_HUD"
)
