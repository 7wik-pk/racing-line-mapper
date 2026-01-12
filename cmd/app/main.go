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

// ============================================================================
// CONFIGURATION - Adjust these values to customize the simulation
// ============================================================================

// Input track file path
const InputTrackPath = "processed_tracks/monza_10m.jpg"

// Render window dimensions
const (
	WindowWidth  = 1200
	WindowHeight = 800
)

// Simulation settings
const (
	TrainingSpeedMultiplier = 3000 // Ticks per frame in training mode (1 = real-time)
	CarSpawnWaypointIndex   = 5    // Which waypoint to spawn the car at (0 = start marker)
	ViewScaleMargin         = 0.95 // Margin for fitting track in window (0.95 = 5% padding)
)

// Track surface colors
var (
	ColorTarmac = color.RGBA{80, 80, 80, 255}
	ColorGravel = color.RGBA{20, 20, 20, 255}
	ColorWall   = color.RGBA{10, 10, 10, 255}
	ColorStart  = color.RGBA{255, 0, 0, 255}
	ColorDir    = color.RGBA{255, 255, 0, 255}
)

// Visualization colors
var (
	ColorFrenetFrame = color.RGBA{50, 155, 50, 40} // Bright Green (was: 100, 200, 255, 150 for Cyan)
	// ColorFrenetFrame = color.RGBA{255, 255, 255, 50} // White
	ColorCar         = color.RGBA{255, 0, 0, 255}    // Red
	ColorCarHeading  = color.RGBA{255, 255, 0, 255}  // Yellow
	ColorBestLap     = color.RGBA{50, 255, 50, 150}  // Light Green
	ColorCurrentLap  = color.RGBA{255, 255, 0, 200}  // Yellow
	ColorLapHistory1 = color.RGBA{255, 0, 255, 255}  // Magenta (most recent)
	ColorLapHistory2 = color.RGBA{190, 0, 190, 150}  // Faded Magenta
	ColorLapHistory3 = color.RGBA{130, 0, 130, 70}   // More Faded
	ColorLapHistory4 = color.RGBA{70, 0, 70, 20}     // Most Faded
)

// ============================================================================

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

	// Rendering Scale
	ViewScale   float32
	ViewOffsetX float32
	ViewOffsetY float32
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
		ticks = TrainingSpeedMultiplier
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
		// cx, cy := int(g.Car.Position.X), int(g.Car.Position.Y)
		// cell := g.Grid.Get(cx, cy)
		// fmt.Printf("[CRASH] Pos: (%.1f, %.1f) Cell: %d\n", g.Car.Position.X, g.Car.Position.Y, cell.Type)

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
	// Draw Track Image
	if g.TrackImage != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(float64(g.ViewScale), float64(g.ViewScale))
		op.GeoM.Translate(float64(g.ViewOffsetX), float64(g.ViewOffsetY))
		screen.DrawImage(g.TrackImage, op)
	}

	// Helper to transform world coordinates to screen coordinates
	toScreen := func(x, y float64) (float32, float32) {
		return float32(x)*g.ViewScale + g.ViewOffsetX, float32(y)*g.ViewScale + g.ViewOffsetY
	}

	// Draw Mesh (Debug)
	if g.Mesh != nil {
		for _, wp := range g.Mesh.Waypoints {
			// Draw Rib (Normal) using ACTUAL track width
			p1x, p1y := toScreen(wp.Position.X-wp.Normal.X*(wp.Width/2), wp.Position.Y-wp.Normal.Y*(wp.Width/2))
			p2x, p2y := toScreen(wp.Position.X+wp.Normal.X*(wp.Width/2), wp.Position.Y+wp.Normal.Y*(wp.Width/2))
			vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 1, ColorFrenetFrame, true)
		}
	}

	// Draw Best Lap Path (Light Green)
	if len(g.BestLapPath) > 1 {
		for j := 0; j < len(g.BestLapPath)-1; j++ {
			p1x, p1y := toScreen(g.BestLapPath[j].X, g.BestLapPath[j].Y)
			p2x, p2y := toScreen(g.BestLapPath[j+1].X, g.BestLapPath[j+1].Y)
			vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 3, ColorBestLap, true)
		}
	}

	// Draw Tracelines (History)
	traceColors := []color.RGBA{
		ColorLapHistory1,
		ColorLapHistory2,
		ColorLapHistory3,
		ColorLapHistory4,
	}

	for i, path := range g.LapHistory {
		col := traceColors[i]
		if len(path) > 1 {
			for j := 0; j < len(path)-1; j++ {
				p1x, p1y := toScreen(path[j].X, path[j].Y)
				p2x, p2y := toScreen(path[j+1].X, path[j+1].Y)
				vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 2, col, true)
			}
		}
	}

	// Draw Current Path (Yellow)
	if len(g.CurrentLapPath) > 1 {
		for j := 0; j < len(g.CurrentLapPath)-1; j++ {
			p1x, p1y := toScreen(g.CurrentLapPath[j].X, g.CurrentLapPath[j].Y)
			p2x, p2y := toScreen(g.CurrentLapPath[j+1].X, g.CurrentLapPath[j+1].Y)
			vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 2, ColorCurrentLap, true)
		}
	}

	if g.Car != nil {
		// Draw Car as Rotated Rectangle
		cosH := math.Cos(g.Car.Heading)
		sinH := math.Sin(g.Car.Heading)
		halfW := g.Car.Width / 2
		halfL := g.Car.Length / 2

		// 4 corners in world space
		worldCorners := [4][2]float64{
			{halfL, halfW},
			{halfL, -halfW},
			{-halfL, -halfW},
			{-halfL, halfW},
		}

		var path vector.Path
		for i, p := range worldCorners {
			// Rotate and Translate in world space
			wx := g.Car.Position.X + p[0]*cosH - p[1]*sinH
			wy := g.Car.Position.Y + p[0]*sinH + p[1]*cosH

			// Transform to screen space
			sx, sy := toScreen(wx, wy)

			if i == 0 {
				path.MoveTo(sx, sy)
			} else {
				path.LineTo(sx, sy)
			}
		}
		path.Close()

		var cs ebiten.ColorScale
		cs.ScaleWithColor(ColorCar)
		vector.FillPath(screen, &path, nil, &vector.DrawPathOptions{
			AntiAlias:  true,
			ColorScale: cs,
		})

		// Draw Heading (Slightly longer than car)
		headX, headY := toScreen(g.Car.Position.X, g.Car.Position.Y)
		tipX, tipY := toScreen(
			g.Car.Position.X+math.Cos(g.Car.Heading)*(g.Car.Length/2+5),
			g.Car.Position.Y+math.Sin(g.Car.Heading)*(g.Car.Length/2+5),
		)
		vector.StrokeLine(screen, headX, headY, tipX, tipY, 2, ColorCarHeading, true)
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
	if g.AIMode {
		panelW := 140.0
		panelH := 150.0
		padding := 10.0

		targetX := float32(WindowWidth) - float32(panelW) - float32(padding)
		targetY := float32(padding)

		vector.FillRect(screen, targetX, 0, float32(panelW), float32(panelH), color.RGBA{0, 0, 0, 180}, true)

		specs := "AGENT PARAMS\n"
		specs += "------------\n"
		specs += g.Agent.DebugInfoStr()

		ebitenutil.DebugPrintAt(screen, specs, int(targetX)+10, int(targetY))
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
	return WindowWidth, WindowHeight // 1024x840
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
				r, gr, b = ColorTarmac.R, ColorTarmac.G, ColorTarmac.B
			case track.CellGravel:
				r, gr, b = ColorGravel.R, ColorGravel.G, ColorGravel.B
			case track.CellWall:
				r, gr, b = ColorWall.R, ColorWall.G, ColorWall.B
			case track.CellStart:
				r, gr, b = ColorStart.R, ColorStart.G, ColorStart.B
			case track.CellDirection:
				r, gr, b = ColorDir.R, ColorDir.G, ColorDir.B
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
	trackPath := InputTrackPath
	grid, mesh, err := track.LoadTrackFromImage(trackPath)
	if err != nil {
		// Fallback to assets/track.png if not found
		trackPath = "assets/track.png"
		grid, mesh, err = track.LoadTrackFromImage(trackPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	trackImg := RenderGrid(grid)

	ebiten.SetWindowSize(WindowWidth, WindowHeight)
	ebiten.SetWindowTitle("Racing Line Mapper")

	// 1. Calculate Scale to fit
	winW, winH := float64(WindowWidth), float64(WindowHeight)
	scaleW := winW / float64(grid.Width)
	scaleH := winH / float64(grid.Height)

	viewScale := float32(scaleW)
	if scaleH < scaleW {
		viewScale = float32(scaleH)
	}
	// Add some margin
	viewScale *= ViewScaleMargin

	// 2. Center the track
	viewOffsetX := (float32(winW) - float32(grid.Width)*viewScale) / 2
	viewOffsetY := (float32(winH) - float32(grid.Height)*viewScale) / 2

	// Spawn car at first waypoint
	startX, startY := 400.0, 110.0
	startHeading := 0.0
	if len(mesh.Waypoints) > 0 {
		// Start at configured waypoint index
		startIdx := CarSpawnWaypointIndex
		if startIdx >= len(mesh.Waypoints) {
			startIdx = 0
		}

		wp := mesh.Waypoints[startIdx]
		startX = wp.Position.X
		startY = wp.Position.Y

		// Align heading with track direction (Normal rotated 90 deg)
		// Normal = (-dy, dx), so Direction = (dx, dy) = (Normal.Y, -Normal.X)
		// Actually, let's just use the vector to the next waypoint
		nextWP := mesh.Waypoints[(startIdx+1)%len(mesh.Waypoints)]
		dx := nextWP.Position.X - wp.Position.X
		dy := nextWP.Position.Y - wp.Position.Y
		startHeading = math.Atan2(dy, dx)
	}

	car := physics.NewCar(startX, startY)
	car.Heading = startHeading
	ag := agent.NewAgent()

	game := &Game{
		Grid:        grid,
		Mesh:        mesh,
		TrackImage:  trackImg,
		Car:         car,
		Agent:       ag,
		AIMode:      true,
		Training:    true,
		ViewScale:   viewScale,
		ViewOffsetX: viewOffsetX,
		ViewOffsetY: viewOffsetY,
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
