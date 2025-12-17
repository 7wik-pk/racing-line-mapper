package main

import (
	"fmt"
	"log"
	"math"
	"racing-line-mapper/internal/agent"
	"racing-line-mapper/internal/physics"
	"racing-line-mapper/internal/track"

	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
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
}

func (g *Game) Update() error {
	if g.Car == nil {
		return nil
	}

	// Toggle AI
	if ebiten.IsKeyPressed(ebiten.KeyA) {
		g.AIMode = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyM) {
		g.AIMode = false
	}

	// Toggle Speed
	// TODO: change to toggle, currently this requires holding S
	ticks := 1
	if ebiten.IsKeyPressed(ebiten.KeyS) {
		ticks = 100 // Speed up training
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
	} else {
		if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
			throttle = 1.0
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
			brake = 1.0
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
			steering = -1.0
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
			steering = 1.0
		}
	}

	// Reset if crashed
	if g.Car.Crashed {
		// Penalty for crashing is handled in Learn step usually, but here we just reset
		// If AI, we need to record the crash state
		if g.AIMode {
			reward := agent.CalculateReward(g.Car, g.Grid, g.Mesh)
			// Next state is irrelevant if terminal, but let's pass current
			g.Agent.Learn(currentState, action, reward, currentState)
		}

		// Auto respawn for AI, Manual for Human
		if g.AIMode || ebiten.IsKeyPressed(ebiten.KeyR) {
			// Respawn at closest waypoint to start
			// TODO ensure starting location is appropriate based on track
			startX, startY := 400.0, 110.0
			if len(g.Mesh.Waypoints) > 0 {
				startX = g.Mesh.Waypoints[0].Position.X
				startY = g.Mesh.Waypoints[0].Position.Y
			}
			g.Car = physics.NewCar(startX, startY)
			g.Car.Heading = 0 // Reset heading too
		}
	} else {
		g.Car.Update(g.Grid, throttle, brake, steering)

		if g.AIMode {
			nextState := agent.DiscretizeState(g.Car, g.Mesh)
			reward := agent.CalculateReward(g.Car, g.Grid, g.Mesh)
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
			vector.FillCircle(screen, float32(wp.Position.X), float32(wp.Position.Y), 2, color.RGBA{0, 255, 255, 255}, true)

			// Draw Rib (Normal)
			p1x := wp.Position.X - wp.Normal.X*20
			p1y := wp.Position.Y - wp.Normal.Y*20
			p2x := wp.Position.X + wp.Normal.X*20
			p2y := wp.Position.Y + wp.Normal.Y*20
			vector.StrokeLine(screen, float32(p1x), float32(p1y), float32(p2x), float32(p2y), 1, color.RGBA{0, 100, 100, 100}, true)
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

	msg := fmt.Sprintf("Mode: %s | Speed: %.2f | Reward: %.2f", "Manual", g.Car.Speed, 0.0)
	if g.AIMode {
		msg = fmt.Sprintf("Mode: AI | Speed: %.2f | Agent-Info: %s", g.Car.Speed, g.Agent.DebugInfoStr())
	}
	if g.Car.Crashed {
		msg += " [CRASHED]"
	}
	msg += "\nControls: A=AI, M=Manual, S=SpeedUp"
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

	if err := ebiten.RunGame(&Game{Grid: grid, Mesh: mesh, TrackImage: trackImg, Car: car, Agent: ag}); err != nil {
		log.Fatal(err)
	}
}
