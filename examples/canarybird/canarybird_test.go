package main

import (
	"testing"

	"graphics.gd/classdb"
	"graphics.gd/classdb/AudioStreamWAV"
	"graphics.gd/classdb/BoxMesh"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Signal"
)

func TestItoa(t *testing.T) {
	cases := map[int]string{
		0: "0", 1: "1", 9: "9", 10: "10", 42: "42", 100: "100",
		-1: "-1", -42: "-42", 1234567: "1234567",
	}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Fatalf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestSynthesisedTone proves the test binary can build a Godot
// resource (AudioStreamWAV) via classdb, push a Go byte slice into
// it through the variant marshaller, and read every primitive
// property back out unchanged. If the cgo bridge or the engine's
// resource subsystem regresses, this fails before any scene work.
func TestSynthesisedTone(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		const hz, secs = 440.0, 0.10
		wantBytes := int(22050*secs) * 2
		stream := synthesisedTone(hz, secs)
		if got := stream.MixRate(); got != 22050 {
			t.Errorf("MixRate = %d, want 22050", got)
		}
		if got := stream.Format(); got != AudioStreamWAV.Format16Bits {
			t.Errorf("Format = %v, want Format16Bits", got)
		}
		if got := len(stream.Data()); got != wantBytes {
			t.Errorf("len(Data) = %d, want %d", got, wantBytes)
		}
	})
}

// TestColoredMaterial proves a Color.RGBA pushed through
// StandardMaterial3D.SetAlbedoColor survives a round-trip through
// the engine's BaseMaterial3D property store.
func TestColoredMaterial(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		want := Color.RGBA{R: 0.25, G: 0.5, B: 0.75, A: 1.0}
		m := coloredMaterial(want)
		got := m.AsBaseMaterial3D().AlbedoColor()
		if !colorApproxEqual(got, want) {
			t.Fatalf("AlbedoColor = %+v, want %+v", got, want)
		}
	})
}

// TestSlab proves nested classdb composition: a BoxMesh wired into
// a MeshInstance3D with a material override and a 3D transform.
// Position and box dimensions are read back through the engine.
func TestSlab(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		mat := coloredMaterial(Color.RGBA{R: 1, G: 1, B: 1, A: 1})
		mi := slab(-5, 0.5, 30, 8, -2, mat)
		pos := mi.AsNode3D().Position()
		if pos.Y != -5 || pos.Z != -2 {
			t.Errorf("Position = %+v, want Y=-5 Z=-2", pos)
		}
		mesh, ok := Object.As[BoxMesh.Instance](mi.Mesh())
		if !ok {
			t.Fatalf("mesh is %T, want BoxMesh.Instance", mi.Mesh())
		}
		size := mesh.Size()
		if size.X != 30 || size.Y != 0.5 || size.Z != 8 {
			t.Errorf("Size = %+v, want (30, 0.5, 8)", size)
		}
	})
}

// TestHighScorePersistence proves the user:// virtual filesystem
// is wired in the test runner: write a high score, then re-read it
// through a fresh ConfigFile. Catches regressions in the
// user-data-dir resolution + ConfigFile codec.
func TestHighScorePersistence(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		const want = 123
		saveHighScore(want)
		t.Cleanup(func() { saveHighScore(0) })
		if got := loadHighScore(); got != want {
			t.Fatalf("loadHighScore = %d, want %d", got, want)
		}
	})
}

// signalProbe is a tiny Node extension whose only purpose is to host
// a Signal.Solo field so the engine's signal bus is exercised the
// same way CanaryBird.Tweeted is — via classdb registration, scene-
// tree attachment, and a Go-defined Callable handler.
type signalProbe struct {
	Node.Extension[signalProbe]
	Pinged Signal.Solo[int]
}

func init() { classdb.Register[signalProbe]() }

// TestSignalEmitsToCallable proves a Signal.Solo[int] declared on a
// registered classdb.Extension carries a Go handler end-to-end
// through the engine's signal bus. Mirrors how CanaryBird.Tweeted
// is fired from flap().
func TestSignalEmitsToCallable(t *testing.T) {
	probe := new(signalProbe)
	runOnMain(t, func(t testing.TB) {
		SceneTree.Add(probe.AsNode())
	})
	got := make(chan int, 1)
	runOnMain(t, func(t testing.TB) {
		probe.Pinged.Call(func(v int) { got <- v })
		probe.Pinged.Emit(42)
	})
	select {
	case v := <-got:
		if v != 42 {
			t.Fatalf("handler received %d, want 42", v)
		}
	default:
		t.Fatal("handler was not invoked synchronously after Emit")
	}
}

func colorApproxEqual(a, b Color.RGBA) bool {
	const eps = 1e-4
	d := func(x, y float32) bool { return x-y < eps && y-x < eps }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}
