package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// GridFamily represents a family of infinite parallel grid lines.
// Each line satisfies n·(x - center) = k*Spacing + Offset for some integer k.
type GridFamily struct {
	Normal    Vec2    // must be normalized
	Spacing   float64 // pixels between lines
	Offset    float64 // pixels along normal from center
	Color     color.Color
	Thickness float64 // half-thickness used for touch detection and drawing width
}

// Game holds the entire app state.
type Game struct {
	W, H int

	Grids   []GridFamily
	Points  []Vec2
	moveDir Vec2    // direction of the moving tiled pattern
	speed   float64 // pixels per second magnitude

	lastInside [][]bool // [gridIdx][pointIdx] whether point was inside thickness band last frame

	// hover/click state
	hoverIdx int // -1 if none hovered

	// audio
	audioCtx       *audio.Context
	blipPCM        []byte
	blipSampleRate int
}

func NewGame() *Game {
	w, h := 960, 640
	// Define some default grids
	grids := []GridFamily{
		{Normal: Vec2{1, 0}.Norm(), Spacing: 60, Offset: 0, Color: color.RGBA{0x66, 0x66, 0xFF, 0xFF}, Thickness: 2},
		{Normal: Vec2{0, 1}.Norm(), Spacing: 60, Offset: 0, Color: color.RGBA{0x66, 0xFF, 0x66, 0xFF}, Thickness: 2},
		{Normal: Vec2{1, 1}.Norm(), Spacing: 85, Offset: 0, Color: color.RGBA{0xFF, 0x66, 0x66, 0xFF}, Thickness: 2},
	}
	// Some fixed points
	points := []Vec2{
		{float64(w) * 0.25, float64(h) * 0.5},
		{float64(w) * 0.5, float64(h) * 0.5},
		{float64(w) * 0.75, float64(h) * 0.5},
		{float64(w) * 0.5, float64(h) * 0.25},
		{float64(w) * 0.5, float64(h) * 0.75},
	}

	last := make([][]bool, len(grids))
	for i := range last {
		last[i] = make([]bool, len(points))
	}

	// Audio context, pick a common sample rate
	const sampleRate = 48000
	ac := audio.NewContext(sampleRate)
	blip := generateBlipPCM(sampleRate, 0.06, 880) // 60ms 880Hz

	return &Game{
		W: w, H: h,
		Grids:          grids,
		Points:         points,
		moveDir:        Vec2{1, 0.3}.Norm(),
		speed:          120, // px/sec
		lastInside:     last,
		hoverIdx:       -1,
		audioCtx:       ac,
		blipPCM:        blip,
		blipSampleRate: sampleRate,
	}
}

func (g *Game) Update() error {
	// Controls: Left/Right rotate direction, Up/Down adjust speed additively
	// Timing
	dt := 1.0 / 60.0 // Ebiten Update is 60 FPS logic

	// Handle mouse hover and click for adding/removing points
	mx, my := ebiten.CursorPosition()
	mouse := Vec2{float64(mx), float64(my)}
	// Hover detection within small radius
	hoverRadius := 10.0
	g.hoverIdx = -1
	bestDist := hoverRadius
	for i, p := range g.Points {
		d := math.Hypot(p.X-mouse.X, p.Y-mouse.Y)
		if d <= bestDist {
			bestDist = d
			g.hoverIdx = i
		}
	}
	// Mouse click handling
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if g.hoverIdx >= 0 {
			// Remove hovered point
			idx := g.hoverIdx
			g.Points = append(g.Points[:idx], g.Points[idx+1:]...)
			for gi := range g.lastInside {
				row := g.lastInside[gi]
				g.lastInside[gi] = append(row[:idx], row[idx+1:]...)
			}
			g.hoverIdx = -1
		} else {
			// Add new point at mouse position
			g.Points = append(g.Points, mouse)
			for gi := range g.lastInside {
				g.lastInside[gi] = append(g.lastInside[gi], false)
			}
		}
	}

	// Rotate movement direction by a fixed angular rate
	rotSpeed := 90.0 * (math.Pi / 180.0) // radians per second
	// Compute current angle from moveDir
	angle := math.Atan2(g.moveDir.Y, g.moveDir.X)
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		angle -= rotSpeed * dt
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		angle += rotSpeed * dt
	}
	g.moveDir = Vec2{math.Cos(angle), math.Sin(angle)}

	// Adjust speed by a fixed amount per second
	accel := 120.0 // px/s^2
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		g.speed += accel * dt
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.speed -= accel * dt
	}
	if g.speed < 0 {
		g.speed = 0
	}

	// Advance offsets based on projection of movement onto grid normals
	step := g.moveDir.Mul(g.speed * dt)
	for i := range g.Grids {
		proj := g.Grids[i].Normal.Dot(step)
		g.Grids[i].Offset += proj
	}

	// Touch detection and blips
	center := Vec2{float64(g.W) / 2, float64(g.H) / 2}
	for gi, gf := range g.Grids {
		th := gf.Thickness
		for pi, p := range g.Points {
			// Compute minimal distance to any grid line of this family that could be close to the point.
			// Distance along normal from center to point.
			dAlong := gf.Normal.Dot(p.Sub(center))
			// Find nearest integer k such that |dAlong - (k*Spacing + Offset)| minimized
			k := math.Round((dAlong - gf.Offset) / gf.Spacing)
			closest := (k * gf.Spacing) + gf.Offset
			d := math.Abs(dAlong - closest)
			inside := d <= th
			if inside && !g.lastInside[gi][pi] {
				g.playBlip()
			}
			g.lastInside[gi][pi] = inside
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Fill background
	screen.Fill(color.RGBA{0x0D, 0x0D, 0x10, 0xFF})

	center := Vec2{float64(g.W) / 2, float64(g.H) / 2}
	diag := math.Hypot(float64(g.W), float64(g.H))
	for _, gf := range g.Grids {
		n := gf.Normal
		t := n.Perp()
		// Determine range of k that fits in window bounds: cover up to diagonal distance
		maxD := diag
		kMin := int(math.Floor((-maxD-gf.Offset)/gf.Spacing)) - 1
		kMax := int(math.Ceil((maxD-gf.Offset)/gf.Spacing)) + 1
		for k := kMin; k <= kMax; k++ {
			d := float64(k)*gf.Spacing + gf.Offset
			pt := center.Add(n.Mul(d))
			p1 := pt.Add(t.Mul(diag))
			p2 := pt.Sub(t.Mul(diag))
			// Draw line
			vector.StrokeLine(screen, float32(p1.X), float32(p1.Y), float32(p2.X), float32(p2.Y), 1.5, gf.Color, true)
		}
	}

	// Draw points
	for i, p := range g.Points {
		if i == g.hoverIdx {
			// highlighted point
			drawCross(screen, p, 8, color.RGBA{0xFF, 0xFF, 0x66, 0xFF})
		} else {
			drawCross(screen, p, 6, color.RGBA{0xFF, 0xEE, 0xAA, 0xFF})
		}
	}

	// HUD text
	msg := "Mouse: Left click add/remove point. Hover to highlight.  "
	msg += "Arrows: Left/Right rotate, Up/Down speed +/-  ESC: quit\n"
	msg += fmt.Sprintf("Speed: %.1f px/s  Dir:(%.2f, %.2f)", g.speed, g.moveDir.X, g.moveDir.Y)
	ebitenutil.DebugPrint(screen, msg)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.W, g.H
}

func drawCross(dst *ebiten.Image, p Vec2, size float64, col color.Color) {
	// Two lines crossing at p
	vector.StrokeLine(dst, float32(p.X-size), float32(p.Y), float32(p.X+size), float32(p.Y), 1.5, col, true)
	vector.StrokeLine(dst, float32(p.X), float32(p.Y-size), float32(p.X), float32(p.Y+size), 1.5, col, true)
}

func (g *Game) playBlip() {
	// Create a new player each trigger to allow overlapping blips
	pl := g.audioCtx.NewPlayerFromBytes(g.blipPCM)
	_ = pl.Rewind()
	pl.Play()
	// Let the player GC when done; ebiten stops it automatically once finished.
}

func main() {
	game := NewGame()
	// Basic window setup
	ebiten.SetWindowSize(game.W, game.H)
	ebiten.SetWindowTitle("Grythm — Grid Rhythm Visualizer")
	if err := ebiten.RunGame(game); err != nil {
		panic(err)
	}
}
