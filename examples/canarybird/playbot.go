package main

import (
	"encoding/json"
	"os"

	"graphics.gd/classdb/CanvasLayer"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
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
}

// playSchedule is the flap timeline in game-seconds from statePlaying.
// Stops short of the deadline so gravity ends the run.
var playSchedule = []Float.X{
	0.05, 0.55, 1.10, 1.65, 2.20, 2.75, 3.30, 3.85, 4.40,
}

func newPlayBot(game *CanaryBird) *playBot {
	Engine.SetMaxFps(60)
	mountDebugOverlay(game)
	return &playBot{game: game, schedule: playSchedule}
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
	report := map[string]any{
		"score":   t.game.score,
		"flaps":   t.flaps,
		"elapsed": t.elapsed,
		"crashed": crashed,
	}
	if path := os.Getenv("GDNEXT_PLAY_REPORT"); path != "" {
		if data, err := json.MarshalIndent(report, "", "  "); err == nil {
			_ = os.WriteFile(path, data, 0644)
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
	path := os.Getenv("GDNEXT_PLAY_SCREENSHOT")
	if path == "" {
		return
	}
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		return
	}
	img := tree.Root().AsViewport().GetTexture().AsTexture2D().GetImage()
	_ = os.WriteFile(path, img.SavePngToBuffer(), 0644)
}

// mountDebugOverlay stamps build metadata (target tuple, link mode,
// host, sha, run-id, ...) into the top-right corner. The text comes
// from $GDNEXT_PLAY_LABEL, which `gdnext ci play-cell` populates from
// its --target/--link flags and the workflow's GITHUB_* env. Skipped
// when the env is empty so the overlay doesn't leak into local runs.
func mountDebugOverlay(game *CanaryBird) {
	text := os.Getenv("GDNEXT_PLAY_LABEL")
	if text == "" {
		return
	}
	layer := CanvasLayer.New()
	game.AsNode().AddChild(layer.AsNode())
	label := Label.New()
	label.SetText(text)
	label.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	ctl := label.AsControl()
	ctl.SetAnchorsPreset(Control.PresetTopRight)
	ctl.SetPosition(Vector2.New[Float.X](-260, 16))
	ctl.SetSize(Vector2.New[Float.X](244, 160))
	layer.AsNode().AddChild(label.AsNode())
}
