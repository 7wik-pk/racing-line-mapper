package main

import (
	"fmt"
	"log"
	"math"
	"racing-line-mapper/internal/agent"
	"racing-line-mapper/internal/common"
	"racing-line-mapper/internal/physics"
	"racing-line-mapper/internal/track"

	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Game struct {
	Grid       *track.Grid
	Mesh       *track.TrackMesh
	TrackImage *ebiten.Image
	Car        *physics.Car
	Agent      agent.Agent
	AIMode     bool
	Training   bool // Fast forward

	// Analytics & Visuals
	NumLaps        int
	BestLapTime    int             // In ticks
	BestLapPath    []common.Vec2   // Path of the best lap
	CurrentLapPath []common.Vec2   // Path of current lap
	LapHistory     [][]common.Vec2 // Paths of last 4 laps
	PreviousLaps   int             // To detect lap change
}

func (g *Game) Update() error {
	if g.Car == nil {
		return nil
	}

	// Toggle AI (Removed Manual Toggle)
	// g.AIMode is always true now

	// Toggle Speed (S now *slows down* from fast training)
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		g.Training = !g.Training
	}

	ticks := 1
	if g.Training {
		ticks = 3000 // Speed up training
	}

	for i := 0; i < ticks; i++ {
		g.updatePhysics()
	}

	return nil
}

func (g *Game) updatePhysics() {
	throttle := 0.0
	brake := 0.0
	steering := 0.0

	// Update Timer
	g.Car.CurrentLapTime++

	// Record Trace (sample every 5 ticks to save memory/drawing)
	if g.Car.CurrentLapTime%5 == 0 {
		g.CurrentLapPath = append(g.CurrentLapPath, g.Car.Position)
	}

	currentState := agent.DiscretizeState(g.Car, g.Mesh)
	action := 0

	if g.AIMode {
		action = g.Agent.SelectAction(currentState)
		switch action {
		case agent.ActionThrottle:
			throttle = 1.0
		case agent.ActionBrake:
			brake = 1.0
		case agent.ActionLeft:
			steering = -1.0
		case agent.ActionRight:
			steering = 1.0
		}
	}

	// Reset if crashed
	if g.Car.Crashed {
		// Penalty for crashing is handled in Learn step usually, but here we just reset
		// If AI, we need to record the crash state
		if g.AIMode {
			reward := agent.CalculateReward(g.Car, g.Grid, g.Mesh, g.BestLapTime)
			// Next state is irrelevant if terminal, but let's pass current
			g.Agent.Learn(currentState, action, reward, currentState)
		}

		// Auto respawn for AI, Manual for Human
		if g.AIMode || ebiten.IsKeyPressed(ebiten.KeyR) {
			// Respawn at closest waypoint to start
			startX, startY := 400.0, 110.0
			if len(g.Mesh.Waypoints) > 0 {
				startX = g.Mesh.Waypoints[0].Position.X
				startY = g.Mesh.Waypoints[0].Position.Y
			}
			g.Car = physics.NewCar(startX, startY)
			g.Car.Heading = 0     // Reset heading too
			g.Car.Checkpoint = -1 // Reset checkpoint
			g.Car.Laps = 0
			// Reset Traces
			g.CurrentLapPath = []common.Vec2{}
			g.PreviousLaps = 0
		}
	} else {
		g.Car.Update(g.Grid, throttle, brake, steering)

		// Check for Lap Completion
		if g.Car.Laps > g.PreviousLaps {
			// Completed a lap!
			g.Car.LastLapTime = g.Car.CurrentLapTime

			// Update Best Time
			if g.BestLapTime == 0 || g.Car.LastLapTime < g.BestLapTime {
				g.BestLapTime = g.Car.LastLapTime
				// Save Best Path (Copy slice)
				g.BestLapPath = make([]common.Vec2, len(g.CurrentLapPath))
				copy(g.BestLapPath, g.CurrentLapPath)
			}

			// Save Trace
			g.LapHistory = append([][]common.Vec2{g.CurrentLapPath}, g.LapHistory...)
			if len(g.LapHistory) > 4 {
				g.LapHistory = g.LapHistory[:4]
			}

			// Reset Current Trace
			g.CurrentLapPath = []common.Vec2{}
			g.Car.CurrentLapTime = 0
			g.PreviousLaps = g.Car.Laps
			g.NumLaps++
		}

		if g.AIMode {
			nextState := agent.DiscretizeState(g.Car, g.Mesh)
			reward := agent.CalculateReward(g.Car, g.Grid, g.Mesh, g.BestLapTime)
			g.Agent.Learn(currentState, action, reward, nextState)
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.TrackImage != nil {
		screen.DrawImage(g.TrackImage, nil)
	}

	// Draw Mesh (Debug)
	if g.Mesh != nil {
		for _, wp := range g.Mesh.Waypoints {
			// Draw Center point
			// vector.FillCircle(screen, float32(wp.Position.X), float32(wp.Position.Y), 2, color.RGBA{0, 255, 255, 255}, true)

			// Draw Rib (Normal)
			p1x := wp.Position.X - wp.Normal.X*20
			p1y := wp.Position.Y - wp.Normal.Y*20
			p2x := wp.Position.X + wp.Normal.X*20
			p2y := wp.Position.Y + wp.Normal.Y*20
			vector.StrokeLine(screen, float32(p1x), float32(p1y), float32(p2x), float32(p2y), 1, color.RGBA{0, 100, 100, 50}, true)
		}
	}

	// Draw Best Lap Path (Light Green)
	if len(g.BestLapPath) > 1 {
		for j := 0; j < len(g.BestLapPath)-1; j++ {
			p1 := g.BestLapPath[j]
			p2 := g.BestLapPath[j+1]
			vector.StrokeLine(screen, float32(p1.X), float32(p1.Y), float32(p2.X), float32(p2.Y), 3, color.RGBA{50, 255, 50, 150}, true)
		}
	}

	// Draw Tracelines (History)
	// Index 0 = Most Recent (Darkest)
	// Colors: use Red/Purple for traces
	traceColors := []color.RGBA{
		{255, 0, 255, 255}, // Magenta Solid
		{190, 0, 190, 150},
		{130, 0, 130, 70},
		{70, 0, 70, 20},
	}

	for i, path := range g.LapHistory {
		col := traceColors[i]
		if len(path) > 1 {
			for j := 0; j < len(path)-1; j++ {
				p1 := path[j]
				p2 := path[j+1]
				vector.StrokeLine(screen, float32(p1.X), float32(p1.Y), float32(p2.X), float32(p2.Y), 2, col, true)
			}
		}
	}

	// Draw Current Path (Yellow)
	if len(g.CurrentLapPath) > 1 {
		for j := 0; j < len(g.CurrentLapPath)-1; j++ {
			p1 := g.CurrentLapPath[j]
			p2 := g.CurrentLapPath[j+1]
			vector.StrokeLine(screen, float32(p1.X), float32(p1.Y), float32(p2.X), float32(p2.Y), 2, color.RGBA{255, 255, 0, 200}, true)
		}
	}

	if g.Car != nil {
		// Draw Car
		vector.FillCircle(screen, float32(g.Car.Position.X), float32(g.Car.Position.Y), 5, color.RGBA{255, 0, 0, 255}, true)

		// Draw Heading
		endX := g.Car.Position.X + math.Cos(g.Car.Heading)*10
		endY := g.Car.Position.Y + math.Sin(g.Car.Heading)*10
		vector.StrokeLine(screen, float32(g.Car.Position.X), float32(g.Car.Position.Y), float32(endX), float32(endY), 2, color.RGBA{255, 255, 0, 255}, true)
	}

	// Draw HUD Background
	// Panel size: 220x100 approx
	// Let's Move the BOX to 0,0 to match DebugPrint.
	vector.FillRect(screen, 0, 0, 140, 200, color.RGBA{0, 0, 0, 180}, true)
	// vector.StrokeRect(screen, 0, 0, 250, 140, 2, color.RGBA{255, 255, 255, 100}, true)

	msg := "STATUS MONITOR\n"
	msg += "----------------\n"
	if g.AIMode {
		msg += "Mode:   AI (Agent)\n"
		msg += fmt.Sprintf("Speed:  %.2f\n", g.Car.Speed)
		msg += fmt.Sprintf("Laps:   %d\n", g.NumLaps)
	} else {
		msg += "Mode:   Manual\n"
	}

	// Time Info
	bestTimeSec := float64(g.BestLapTime) / 60.0
	lastTimeSec := float64(g.Car.LastLapTime) / 60.0
	currTimeSec := float64(g.Car.CurrentLapTime) / 60.0

	msg += fmt.Sprintf("Current: %.2fs\n", currTimeSec)
	msg += fmt.Sprintf("Last:    %.2fs\n", lastTimeSec)
	msg += fmt.Sprintf("Best:    %.2fs\n", bestTimeSec)

	// Draw Agent Specs Panel (Top Right)
	// 550, 0 position
	if g.AIMode {
		vector.FillRect(screen, 650, 0, 140, 150, color.RGBA{0, 0, 0, 180}, true)

		specs := "AGENT PARAMS\n"
		specs += "------------\n"
		specs += g.Agent.DebugInfoStr()

		// Draw at 560, 10 (approx via spacing hack or just Print)
		// Since DebugPrint is at 0,0, we need a way to draw text at X,Y.
		// Standard Ebiten doesn't make this easy without loading a font face.
		// However, we can trick it by using `ebitenutil.DebugPrintAt` which DOES exist in recent versions?
		// Let's assume it does not.
		// If I cannot use DebugPrintAt, I will append to the main block but that ruins the "top right" requirement.

		ebitenutil.DebugPrintAt(screen, specs, 660, 10)
	}

	if g.Car.Crashed {
		msg += " [CRASHED]"
	}
	if g.Training {
		msg += " [High speed]"
	} else {
		msg += " [Real-time speed]"
	}
	msg += "\nControls:\nS = Toggle Slow Mode"

	// Position text with padding inside the box
	// ebitenutil.DebugPrint draws at 0,0 by default.
	// We'll wrap it in a Translate? No, DebugPrint is extremely simple.
	// We'll use DebugPrintAt to position it. But we don't have that imported?
	// DebugPrint supports newlines but fixed pos.
	// Actually, DebugPrint always draws at 0,0.
	// Let's rely on the box being at 0,0 approx (10,10).
	// We need to move the TEXT to 15,15.
	// `ebitenutil.DebugPrintAt` exists? Let's check imports.
	// ebitenutil is imported.
	// Let's assume DebugPrintAt is not available in standard ebitenutil?
	// Wait, standard ebitenutil ONLY has DebugPrint.
	// Creating a custom PrintAt using DebugPrint is impossible because it hardcodes position.
	// I should use `text.Draw` if I want position.
	// But I don't want to load fonts right now.
	// Hack: Pad the string with spaces/newlines?
	// Or just draw the box at 0,0.

	ebitenutil.DebugPrint(screen, msg)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 800, 600
}

func RenderGrid(g *track.Grid) *ebiten.Image {
	img := ebiten.NewImage(g.Width, g.Height)
	// We can map pixels directly
	// For performance in Ebiten, it's better to use ReplacePixels or similar if we have the byte slice
	// But since our Grid is a struct of Cells, we iterate.
	// Optimization: Grid should probably hold a byte slice for the visual layer to avoid this loop every time we load - could make logic involving coords elsewhere harder to code.

	pixels := make([]byte, g.Width*g.Height*4)
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			cell := g.Get(x, y)
			idx := (y*g.Width + x) * 4

			var r, gr, b byte
			switch cell.Type {
			case track.CellTarmac:
				r, gr, b = 50, 50, 50 // Dark Gray
			case track.CellGravel:
				r, gr, b = 0, 200, 0 // Green
			case track.CellWall:
				r, gr, b = 255, 255, 255 // White
			case track.CellStart:
				r, gr, b = 200, 0, 0 // Red
			}

			pixels[idx] = r
			pixels[idx+1] = gr
			pixels[idx+2] = b
			pixels[idx+3] = 255
		}
	}

	img.WritePixels(pixels)
	return img
}

func main() {
	grid, mesh, err := track.LoadTrackFromImage("assets/track.png")
	if err != nil {
		log.Fatal(err)
	}

	trackImg := RenderGrid(grid)

	// TODO: figure out how to dynamically display one part of a big track - for example, the Nurburgring can't be shown in an 800x600 window as it's very big - the car will be barely visible.
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("Racing Line Mapper")

	// Spawn car at first waypoint
	startX, startY := 400.0, 110.0
	if len(mesh.Waypoints) > 0 {
		startX = mesh.Waypoints[0].Position.X
		startY = mesh.Waypoints[0].Position.Y
	}

	car := physics.NewCar(startX, startY)
	ag := agent.NewAgent()

	if err := ebiten.RunGame(&Game{Grid: grid, Mesh: mesh, TrackImage: trackImg, Car: car, Agent: ag, AIMode: true, Training: false}); err != nil {
		log.Fatal(err)
	}
}
