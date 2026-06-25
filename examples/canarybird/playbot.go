package main

import (
	"encoding/json"
	"fmt"
	"os"

	"graphics.gd/classdb/CanvasLayer"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/GridContainer"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/product"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
)

// playBot scripts canarybird from a fixed flap schedule when
// GDNEXT_PLAY is set. Combined with the game's seeded RNG and a
// pinned 60 fps cap, the run is deterministic across hosts.
type playBot struct {
	game     *CanaryBird
	elapsed  Float.X
	schedule []Float.X
	next     int
	pressed  bool
	holdEnd  Float.X
	flaps    int
	exited   bool
	hudCols  []product.PlayHUDColumn
}

// playSchedule is the flap timeline in game-seconds from statePlaying.
// Stops short of the deadline so gravity ends the run.
var playSchedule = []Float.X{
	0.05, 0.55, 1.10, 1.65, 2.20, 2.75, 3.30, 3.85, 4.40,
}

func newPlayBot(game *CanaryBird) *playBot {
	Engine.SetMaxFps(60)
	bot := &playBot{game: game, schedule: playSchedule}
	bot.hudCols = mountDebugOverlay(game)
	return bot
}

func (t *playBot) tick(delta Float.X) {
	if t.exited {
		return
	}
	t.elapsed += delta
	if t.pressed && t.elapsed >= t.holdEnd {
		Input.ActionRelease("flap")
		t.pressed = false
	}
	switch t.game.state {
	case stateReady:
		if !t.pressed && t.elapsed > 0.1 {
			t.press()
		}
	case statePlaying:
		if t.next < len(t.schedule) && t.elapsed >= t.schedule[t.next] && !t.pressed {
			t.press()
			t.next++
		}
	}
	if t.game.state == stateOver || t.elapsed >= 12.0 {
		t.finish(t.game.state == stateOver)
	}
}

func (t *playBot) press() {
	Input.ActionPress("flap")
	t.pressed = true
	t.holdEnd = t.elapsed + 0.05
	t.flaps++
}

func (t *playBot) finish(crashed bool) {
	t.exited = true
	gameData := map[string]any{
		"score":   t.game.score,
		"flaps":   t.flaps,
		"elapsed": t.elapsed,
		"crashed": crashed,
	}
	// canarybird passes when the bot reaches gameOver via the floor
	// (gravity path) with at least one cloud passed under bot control.
	success := crashed && t.game.score >= 1 && t.elapsed >= 1.0
	report := map[string]any{
		"success":   success,
		"game_data": gameData,
		"hud_data":  t.hudCols,
	}
	if path := os.Getenv(product.EnvPlayReport); path != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			panic(fmt.Errorf("marshal play report: %w", err))
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			panic(fmt.Errorf("write play report %s: %w", path, err))
		}
	}
	t.snapshot()
	if tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop()); ok {
		tree.Quit()
	}
}

// snapshot writes a PNG of the root viewport's current frame to
// $GDNEXT_PLAY_SCREENSHOT when set. SavePngToBuffer + os.WriteFile is
// used (rather than Image.SavePng with a `user://...` path) so the CI
// driver can hand any absolute host path it owns and pick the file up
// from there directly.
func (t *playBot) snapshot() {
	path := os.Getenv(product.EnvPlayScreenshot)
	if path == "" {
		return
	}
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		panic("play screenshot requested but engine main loop is not a SceneTree")
	}
	data := tree.Root().AsViewport().GetTexture().AsTexture2D().GetImage().SavePngToBuffer()
	if err := os.WriteFile(path, data, 0644); err != nil {
		panic(fmt.Errorf("write play screenshot %s: %w", path, err))
	}
}

// mountDebugOverlay renders a one-row table along the bottom of the
// viewport with build/CI provenance and returns the fully-populated
// column slice (Godot Version prepended) so the play report can
// archive the same HUD next to the game data. Columns come from
// $GDNEXT_PLAY_HUD (JSON, supplied by `gdnext ci play-cell`).
// Returns nil when the env is empty so the overlay doesn't leak
// into local runs.
func mountDebugOverlay(game *CanaryBird) []product.PlayHUDColumn {
	raw := os.Getenv(product.EnvPlayHUD)
	if raw == "" {
		return nil
	}
	var cols []product.PlayHUDColumn
	if err := json.Unmarshal([]byte(raw), &cols); err != nil {
		panic(fmt.Errorf("parse %s: %w", product.EnvPlayHUD, err))
	}
	cols = append([]product.PlayHUDColumn{{Name: "Godot Version", Value: Engine.GetVersionInfo().String}}, cols...)
	layer := CanvasLayer.New()
	game.AsNode().AddChild(layer.AsNode())
	panel := PanelContainer.New()
	pctl := panel.AsControl()
	pctl.SetAnchorsPreset(Control.PresetBottomWide)
	pctl.SetOffsetTop(-56)
	pctl.SetMouseFilter(Control.MouseFilterIgnore)
	layer.AsNode().AddChild(panel.AsNode())
	grid := GridContainer.New()
	grid.SetColumns(len(cols))
	grid.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	panel.AsNode().AddChild(grid.AsNode())
	for _, c := range cols {
		h := Label.New()
		h.SetText(c.Name)
		h.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
		hctl := h.AsControl()
		hctl.SetMouseFilter(Control.MouseFilterIgnore)
		hctl.SetSizeFlagsHorizontal(Control.SizeExpandFill)
		grid.AsNode().AddChild(h.AsNode())
	}
	for _, c := range cols {
		v := c.Value
		if v == "" {
			v = "—"
		}
		l := Label.New()
		l.SetText(v)
		l.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
		lctl := l.AsControl()
		lctl.SetMouseFilter(Control.MouseFilterIgnore)
		lctl.SetSizeFlagsHorizontal(Control.SizeExpandFill)
		grid.AsNode().AddChild(l.AsNode())
	}
	return cols
}
