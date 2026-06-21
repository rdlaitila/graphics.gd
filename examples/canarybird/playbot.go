package main

import (
	"encoding/json"
	"os"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/SceneTree"
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
}

// playSchedule is the flap timeline in game-seconds from statePlaying.
// Stops short of the deadline so gravity ends the run.
var playSchedule = []Float.X{
	0.05, 0.55, 1.10, 1.65, 2.20, 2.75, 3.30, 3.85, 4.40,
}

func newPlayBot(game *CanaryBird) *playBot {
	Engine.SetMaxFps(60)
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
	if tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop()); ok {
		tree.Quit()
	}
}
