package physics

import (
	"math"
	"racing-line-mapper/internal/common"
	"racing-line-mapper/internal/track"
)

const (
	MaxSpeed         = 10.0 // Pixels per tick (approx)
	Acceleration     = 0.2
	Braking          = 0.4
	Friction         = 0.05 // Air resistance / Rolling resistance
	TurnSpeed        = 0.05 // Radians per tick
	OffTrackFriction = 0.2  // Extra drag when on gravel
)

type Car struct {
	Position common.Vec2
	Velocity common.Vec2
	Heading  float64 // Radians
	Speed    float64 // Scalar speed (forward/backward)
	Crashed  bool

	// Dimensions (in pixels)
	Width  float64
	Length float64

	// Race State
	Checkpoint     int // Index of the last passed waypoint
	Laps           int
	CurrentLapTime int // Ticks for current lap
	LastLapTime    int // Ticks for previous lap
}

func NewCar(x, y float64) *Car {
	return &Car{
		Position:       common.Vec2{X: x, Y: y},
		Heading:        0,
		Width:          10,   // ~2 meters at 0.2m/px
		Length:         22.5, // ~4.5 meters at 0.2m/px
		Checkpoint:     -1,   // Not started
		LastLapTime:    0,
		CurrentLapTime: 0,
	}
}

// Update advances the car physics.
// throttle: 0.0 to 1.0
// brake: 0.0 to 1.0
// steering: -1.0 (left) to 1.0 (right)
func (c *Car) Update(grid *track.Grid, throttle, brake, steering float64) {
	if c.Crashed {
		return
	}

	// 1. Apply Input
	if throttle > 0 {
		c.Speed += throttle * Acceleration
	}
	if brake > 0 {
		c.Speed -= brake * Braking
	}

	// 2. Apply Drag/Friction (Natural deceleration)
	if c.Speed > 0 {
		c.Speed -= Friction
		if c.Speed < 0 {
			c.Speed = 0
		}
	} else if c.Speed < 0 {
		c.Speed += Friction
		if c.Speed > 0 {
			c.Speed = 0
		}
	}

	// 3. Steering
	// Only steer if moving
	if math.Abs(c.Speed) > 0.1 {
		c.Heading += steering * TurnSpeed
	}

	// 4. Calculate Velocity Vector based on Heading
	// Note: This is "Arcade" physics. Velocity is locked to heading + drift.
	// For true drift, we'd update Velocity separately from Heading.
	// Let's do a simple inertia model:
	// Target Velocity is (Cos(Heading), Sin(Heading)) * Speed
	targetVx := math.Cos(c.Heading) * c.Speed
	targetVy := math.Sin(c.Heading) * c.Speed

	// Lerp towards target velocity (simulates grip)
	// Lower factor = more drift/ice. Higher factor = more grip.
	grip := 0.9

	// 4. Update Position
	newPos := common.Vec2{
		X: c.Position.X + c.Velocity.X,
		Y: c.Position.Y + c.Velocity.Y,
	}

	// 5. Calculate Corners for Collision Detection
	halfW := c.Width / 2
	halfL := c.Length / 2
	cosH := math.Cos(c.Heading)
	sinH := math.Sin(c.Heading)

	// Local corner offsets
	offsets := []common.Vec2{
		{X: halfL, Y: halfW},   // Front Right
		{X: halfL, Y: -halfW},  // Front Left
		{X: -halfL, Y: halfW},  // Rear Right
		{X: -halfL, Y: -halfW}, // Rear Left
	}

	grip = 0.9
	onGravel := false

	for _, off := range offsets {
		// Rotate and translate corner
		worldX := newPos.X + off.X*cosH - off.Y*sinH
		worldY := newPos.Y + off.X*sinH + off.Y*cosH

		cellX := int(worldX)
		cellY := int(worldY)
		cell := grid.Get(cellX, cellY)

		switch cell.Type {
		case track.CellWall:
			c.Crashed = true
			c.Speed = 0
			return
		case track.CellGravel:
			onGravel = true
		}
	}

	if onGravel {
		grip = 0.5
		c.Speed *= (1.0 - OffTrackFriction) // Slow down on gravel
	}

	// Apply final movements
	c.Position = newPos
	c.Velocity.X = c.Velocity.X*(1-grip) + targetVx*grip
	c.Velocity.Y = c.Velocity.Y*(1-grip) + targetVy*grip

	// Clamp speed
	if c.Speed > MaxSpeed {
		c.Speed = MaxSpeed
	}
}
