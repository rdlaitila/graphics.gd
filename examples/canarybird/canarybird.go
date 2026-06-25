package main

import (
	"math"
	"math/rand/v2"
	"os"

	"graphics.gd/classdb/AudioStreamPlayer"
	"graphics.gd/classdb/AudioStreamWAV"
	"graphics.gd/classdb/BoxMesh"
	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Camera3D"
	"graphics.gd/classdb/CanvasLayer"
	"graphics.gd/classdb/ConfigFile"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DirectionalLight3D"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/MeshInstance3D"
	"graphics.gd/classdb/Node3D"
	"graphics.gd/classdb/OmniLight3D"
	"graphics.gd/classdb/SphereMesh"
	"graphics.gd/classdb/StandardMaterial3D"
	"graphics.gd/product"
	"graphics.gd/variant/Angle"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Euler"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Signal"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector3"
)

// Game constants. Hand-tuned for a roughly two-second skill window per
// cloud — short enough to make a CI run finish quickly even if a future
// integration test ever drives the game programmatically.
const (
	gravity       Float.X = 22.0
	flapImpulse   Float.X = 8.5
	scrollSpeed   Float.X = 6.0
	floorY        Float.X = -4.0
	ceilingY      Float.X = 5.0
	cloudCount            = 4
	cloudSpacing  Float.X = 7.0
	cloudKillDist Float.X = 1.25 // bird radius 0.45 + cloud radius 0.9 - a small fudge for forgiveness
	cloudSpawnX   Float.X = 14.0
	cloudDespawnX Float.X = -10.0
	settingsPath          = "user://canarybird.cfg"
)

type gameState int

const (
	stateReady gameState = iota
	statePlaying
	stateOver
)

// CanaryBird is the entire game packed into one Node3D extension. It owns
// the bird, the cloud pool, the lights, the camera, and the HUD; building
// every node from Go demonstrates the classdb code path that all the
// gdnext build targets must support.
type CanaryBird struct {
	Node3D.Extension[CanaryBird] `gd:"CanaryBird"`
	Tweeted                      Signal.Solo[int]
	bird                         MeshInstance3D.Instance
	velocity                     Float.X
	clouds                       [cloudCount]MeshInstance3D.Instance
	flapSfx, crashSfx            AudioStreamPlayer.Instance
	scoreLabel, statusLabel      Label.Instance
	retryButton                  Button.Instance
	score, highScore             int
	state                        gameState
	rng                          *rand.Rand
	bot                          *playBot
}

func (g *CanaryBird) Ready() {
	g.rng = rand.New(rand.NewPCG(0xCA, 0x71))
	g.highScore = loadHighScore()
	g.buildScene()
	g.buildHUD()
	g.reset()
	if os.Getenv(product.EnvPlay) != "" {
		g.bot = newPlayBot(g)
	}
}

func (g *CanaryBird) buildScene() {
	// Sun: keys the scene with a warm overhead light.
	sun := DirectionalLight3D.New()
	sun.AsLight3D().SetLightColor(Color.RGBA{R: 1.0, G: 0.97, B: 0.85, A: 1.0})
	sun.AsLight3D().SetLightEnergy(1.4)
	g.AsNode().AddChild(sun.AsNode())
	sun.AsNode3D().Rotate(Vector3.New[Float.X](1, 0, 0), Angle.Radians(-math.Pi/3))
	// Warm fill from below so the canary's underside isn't pitch black.
	fill := OmniLight3D.New()
	fill.AsLight3D().SetLightColor(Color.RGBA{R: 1.0, G: 0.85, B: 0.55, A: 1.0})
	fill.AsLight3D().SetLightEnergy(1.0)
	g.AsNode().AddChild(fill.AsNode())
	fill.AsNode3D().SetPosition(Vector3.New[Float.X](-2, -3, 4))
	// Camera: side-on at z=12. Default Camera3D orientation looks down -Z,
	// which is what we want — calling LookAt before AddChild errors out
	// because the node isn't in the tree yet, so we just skip it.
	cam := Camera3D.New()
	g.AsNode().AddChild(cam.AsNode())
	cam.AsNode3D().SetPosition(Vector3.New[Float.X](0, 0, 12))
	// Ground slab: wide and deep enough to fill the entire lower half of
	// the camera view. Top face coincides with floorY — what the player
	// sees as "the ground" is exactly the kill plane the game-loop checks.
	// No matching ceiling slab: the upper bound is a soft clamp in tick()
	// (real Flappy Bird also lets you bonk the top harmlessly).
	groundMat := coloredMaterial(Color.RGBA{R: 0.30, G: 0.55, B: 0.20, A: 1.0})
	g.AsNode().AddChild(slab(floorY-0.5, 1.0, 200, 200, -90, groundMat).AsNode())
	// The canary itself — a yellow sphere that doubles as the player.
	g.bird = MeshInstance3D.New()
	birdMesh := SphereMesh.New()
	birdMesh.SetRadius(0.45)
	birdMesh.SetHeight(0.9)
	g.bird.SetMesh(birdMesh.AsMesh())
	g.bird.AsGeometryInstance3D().SetMaterialOverride(coloredMaterial(Color.RGBA{R: 1.0, G: 0.85, B: 0.15, A: 1.0}).AsMaterial())
	g.AsNode().AddChild(g.bird.AsNode())
	g.bird.AsNode3D().SetPosition(Vector3.New[Float.X](-3, 0, 0))
	// Cloud pool: a fixed ring of obstacles we recycle as the bird flies.
	// Pre-allocating keeps the build deterministic — no allocations once
	// the game loop is running.
	cloudMesh := SphereMesh.New()
	cloudMesh.SetRadius(0.9)
	cloudMesh.SetHeight(1.8)
	cloudMat := coloredMaterial(Color.RGBA{R: 0.96, G: 0.96, B: 1.0, A: 1.0})
	for i := range g.clouds {
		c := MeshInstance3D.New()
		c.SetMesh(cloudMesh.AsMesh())
		c.AsGeometryInstance3D().SetMaterialOverride(cloudMat.AsMaterial())
		g.AsNode().AddChild(c.AsNode())
		g.clouds[i] = c
	}
	// Audio: a short upward chirp on flap, a low growl on crash. Both are
	// synthesised at runtime so the example carries no committed audio.
	g.flapSfx = AudioStreamPlayer.New()
	g.flapSfx.SetStream(synthesisedTone(1800.0, 0.08).AsAudioStream())
	g.AsNode().AddChild(g.flapSfx.AsNode())
	g.crashSfx = AudioStreamPlayer.New()
	g.crashSfx.SetStream(synthesisedTone(180.0, 0.4).AsAudioStream())
	g.AsNode().AddChild(g.crashSfx.AsNode())
}

func (g *CanaryBird) buildHUD() {
	layer := CanvasLayer.New()
	g.AsNode().AddChild(layer.AsNode())
	g.scoreLabel = Label.New()
	g.scoreLabel.AsControl().SetAnchorsPreset(Control.PresetTopLeft)
	g.scoreLabel.AsControl().SetPosition(Vector2.New[Float.X](16, 16))
	layer.AsNode().AddChild(g.scoreLabel.AsNode())
	g.statusLabel = Label.New()
	g.statusLabel.AsControl().SetAnchorsPreset(Control.PresetCenterTop)
	g.statusLabel.AsControl().SetPosition(Vector2.New[Float.X](-110, 60))
	g.statusLabel.AsControl().SetSize(Vector2.New[Float.X](220, 24))
	layer.AsNode().AddChild(g.statusLabel.AsNode())
	g.retryButton = Button.New()
	g.retryButton.SetText("Retry")
	g.retryButton.AsControl().SetAnchorsPreset(Control.PresetCenterTop)
	g.retryButton.AsControl().SetPosition(Vector2.New[Float.X](-40, 96))
	g.retryButton.AsBaseButton().OnPressed(g.reset)
	g.retryButton.AsCanvasItem().SetVisible(false)
	layer.AsNode().AddChild(g.retryButton.AsNode())
}

// Process is the game loop: gravity, input, cloud scrolling, collision,
// scoring. Per-frame logic in Go demonstrates the Process callback round-
// trip without an AnimationPlayer or PhysicsBody.
func (g *CanaryBird) Process(delta Float.X) {
	if g.bot != nil {
		g.bot.tick(delta)
	}
	if Input.IsActionJustPressed("flap", false) {
		g.flap()
	}
	switch g.state {
	case stateReady:
		// Idle bob until the first flap kicks the game into motion.
		bob := Float.X(math.Sin(2*math.Pi*0.5*float64(g.simTime()))) * 0.2
		g.bird.AsNode3D().SetPosition(Vector3.New(-3, bob, 0))
	case statePlaying:
		g.tick(delta)
	case stateOver:
		// Frozen world; only the retry button responds.
	}
}

func (g *CanaryBird) tick(delta Float.X) {
	// Bird physics. Soft-clamp the top so flying upward can't kill you;
	// only the floor is fatal.
	g.velocity -= gravity * delta
	pos := g.bird.AsNode3D().Position()
	pos.Y += g.velocity * delta
	if pos.Y > ceilingY {
		pos.Y = ceilingY
		g.velocity = 0
	}
	g.bird.AsNode3D().SetPosition(pos)
	// Pitch the bird down as it falls and up as it rises — purely cosmetic
	// but makes the per-frame Process callback visibly drive the transform.
	tilt := Angle.Radians(math.Atan(float64(g.velocity / 20.0)))
	g.bird.AsNode3D().SetRotation(Euler.Radians{Z: tilt})
	if pos.Y < floorY {
		g.gameOver()
		return
	}
	// Scroll clouds; recycle past the despawn line. Each recycle counts as
	// a successful pass and bumps the score.
	for _, c := range g.clouds {
		cp := c.AsNode3D().Position()
		cp.X -= scrollSpeed * delta
		if cp.X < cloudDespawnX {
			cp.X += cloudSpacing * cloudCount
			cp.Y = Float.X(g.rng.Float64()*8.0 - 3.0)
			g.score++
			g.scoreLabel.SetText("Score: " + itoa(g.score) + "  (best " + itoa(g.highScore) + ")")
		}
		c.AsNode3D().SetPosition(cp)
		// Collision: simple distance-to-cloud-center check. No physics
		// servers needed, which keeps the canary buildable on every
		// runner-supported renderer.
		dx, dy := cp.X-pos.X, cp.Y-pos.Y
		if Float.X(math.Hypot(float64(dx), float64(dy))) < cloudKillDist {
			g.gameOver()
			return
		}
	}
}

func (g *CanaryBird) flap() {
	switch g.state {
	case stateReady:
		g.state = statePlaying
	case stateOver:
		return
	}
	g.velocity = flapImpulse
	g.flapSfx.Play()
	g.Tweeted.Emit(g.score)
}

func (g *CanaryBird) gameOver() {
	g.state = stateOver
	g.crashSfx.Play()
	if g.score > g.highScore {
		g.highScore = g.score
		saveHighScore(g.highScore)
	}
	g.statusLabel.SetText("Game over! Best: " + itoa(g.highScore))
	g.retryButton.AsCanvasItem().SetVisible(true)
}

func (g *CanaryBird) reset() {
	g.state = stateReady
	g.velocity = 0
	g.score = 0
	g.bird.AsNode3D().SetPosition(Vector3.New[Float.X](-3, 0, 0))
	g.bird.AsNode3D().SetRotation(Euler.Radians{})
	g.statusLabel.SetText("Press SPACE to flap!")
	g.retryButton.AsCanvasItem().SetVisible(false)
	g.scoreLabel.SetText("Score: 0  (best " + itoa(g.highScore) + ")")
	// Reseed the cloud ring across the playable strip.
	for i, c := range g.clouds {
		x := cloudSpawnX + Float.X(i)*cloudSpacing
		y := Float.X(g.rng.Float64()*8.0 - 3.0)
		c.AsNode3D().SetPosition(Vector3.New(x, y, Float.X(0)))
	}
}

func (g *CanaryBird) simTime() Float.X {
	return Float.X(g.score) // good enough for the idle-bob phase shift
}

// coloredMaterial returns a basic albedo material in the requested colour.
// The canary uses one of these per logical surface so the example
// exercises StandardMaterial3D + the GeometryInstance3D override path.
func coloredMaterial(c Color.RGBA) StandardMaterial3D.Instance {
	m := StandardMaterial3D.New()
	m.AsBaseMaterial3D().SetAlbedoColor(c)
	return m
}

// slab returns a wide BoxMesh used for the visible ground. (width, depth,
// centreZ) let the caller push it deep into the background so the camera
// only sees its top face — no sky shows through underneath.
func slab(centreY, thickness, width, depth, centreZ Float.X, mat StandardMaterial3D.Instance) MeshInstance3D.Instance {
	mesh := BoxMesh.New()
	mesh.SetSize(Vector3.New(width, thickness, depth))
	mi := MeshInstance3D.New()
	mi.SetMesh(mesh.AsMesh())
	mi.AsGeometryInstance3D().SetMaterialOverride(mat.AsMaterial())
	mi.AsNode3D().SetPosition(Vector3.New(0, centreY, centreZ))
	return mi
}

// synthesisedTone returns an AudioStreamWAV containing a half-volume sine
// wave at hz for the given duration, sampled at 22050 Hz / 16-bit signed
// PCM. All audio in canarybird is generated this way so the repo carries
// no .ogg or .wav files.
func synthesisedTone(hz, seconds float64) AudioStreamWAV.Instance {
	const mixRate = 22050
	samples := int(float64(mixRate) * seconds)
	pcm := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		v := math.Sin(2 * math.Pi * hz * float64(i) / mixRate)
		s := int16(v * 16000)
		pcm[i*2] = byte(s)
		pcm[i*2+1] = byte(s >> 8)
	}
	stream := AudioStreamWAV.New()
	stream.SetFormat(AudioStreamWAV.Format16Bits)
	stream.SetMixRate(mixRate)
	stream.SetData(pcm)
	return stream
}

func loadHighScore() int {
	cfg := ConfigFile.New()
	if err := cfg.Load(settingsPath); err != nil {
		return 0
	}
	if v, ok := cfg.GetValue("canarybird", "high_score").(int64); ok && v >= 0 {
		return int(v)
	}
	return 0
}

func saveHighScore(score int) {
	cfg := ConfigFile.New()
	_ = cfg.Load(settingsPath)
	cfg.SetValue("canarybird", "high_score", int64(score))
	cfg.Save(settingsPath)
}

// itoa is a fmt-free integer-to-string helper. Avoiding fmt shaves a few
// hundred KB off the cgo c-shared output that ships in every release.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
