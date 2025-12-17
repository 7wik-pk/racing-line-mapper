package agent

import (
	"fmt"
	"math"
	"math/rand"
	"racing-line-mapper/internal/physics"
	"racing-line-mapper/internal/track"
)

// Actions
const (
	ActionCoast = iota
	ActionThrottle
	ActionBrake
	ActionLeft
	ActionRight
	ActionCount
)

// Hyperparameters
const (
	Alpha   = 0.01 // Learning Rate
	Gamma   = 0.9998 // Discount Factor
	MinEpsilon = 0.01
	Decay = 0.9995 // Decay Rate
)

var Epsilon = 1.0

// State represents the discretized state of the car.
type State struct {
	SegmentIdx int // Progress along track (0..N)
	LaneIdx    int // Lateral offset (-3..3)
	SpeedLevel int // 0: Stopped, 1: Slow, 2: Medium, 3: Fast
	HeadingRel int // Relative heading to track direction (-2..2)
}

// QTable stores the Q-values for state-action pairs.
type QTable map[State][ActionCount]float64

type Agent interface {
	SelectAction(state State) int
	Learn(state State, action int, reward float64, nextState State)
	DebugInfoStr() string
}

type AgentQTable struct {
	QTable QTable
}

func NewAgent() Agent {
	return &AgentQTable{
		QTable: make(QTable),
	}
}

// DiscretizeState converts continuous car physics to a discrete State.
func DiscretizeState(c *physics.Car, mesh *track.TrackMesh) State {
	// 1. Get Frenet Coordinates
	wp, wpIdx := mesh.GetClosestWaypoint(c.Position)

	// Calculate Lateral Offset (d)
	// Vector from Waypoint to Car
	dx := c.Position.X - wp.Position.X
	dy := c.Position.Y - wp.Position.Y

	// Project onto Normal
	d := dx*wp.Normal.X + dy*wp.Normal.Y

	// Discretize Lane (Track Width approx 50)
	// Center = 0. Width/2 = 25.
	// Lanes: -20..-10, -10..0, 0..10, 10..20
	lane := 0
	if d < -15 {
		lane = -2
	} else if d < -5 {
		lane = -1
	} else if d < 5 {
		lane = 0
	} else if d < 15 {
		lane = 1
	} else {
		lane = 2
	}

	// 2. Speed
	speedLevel := 0
	if c.Speed > 8 {
		speedLevel = 3
	} else if c.Speed > 4 {
		speedLevel = 2
	} else if c.Speed > 0.5 {
		speedLevel = 1
	}

	// 3. Relative Heading
	// Car Heading vs Track Tangent
	// Tangent is Normal rotated -90 deg
	tangentX := wp.Normal.Y
	tangentY := -wp.Normal.X
	trackHeading := math.Atan2(tangentY, tangentX)

	relHeading := c.Heading - trackHeading
	// Normalize -Pi to Pi
	for relHeading > math.Pi {
		relHeading -= 2 * math.Pi
	}
	for relHeading < -math.Pi {
		relHeading += 2 * math.Pi
	}

	// Discretize: -30deg, 0, +30deg
	h := 0
	deg30 := math.Pi / 6
	if relHeading < -deg30 {
		h = -1
	} else if relHeading > deg30 {
		h = 1
	}

	return State{
		SegmentIdx: wpIdx / 5, // Downsample segments (reduce state space)
		LaneIdx:    lane,
		SpeedLevel: speedLevel,
		HeadingRel: h,
	}
}

// SelectAction chooses an action using Epsilon-Greedy policy.
func (a *AgentQTable) SelectAction(state State) int {

	Epsilon = math.Max(Epsilon * Decay, MinEpsilon)

	if rand.Float64() < Epsilon {
		return rand.Intn(ActionCount)
	}

	// Greedy: Find max Q
	qValues, exists := a.QTable[state]
	if !exists {
		return rand.Intn(ActionCount) // Unknown state, explore
	}

	bestAction := 0
	maxQ := -math.MaxFloat64

	// Random tie-breaking
	start := rand.Intn(ActionCount)
	for i := 0; i < ActionCount; i++ {
		idx := (start + i) % ActionCount
		if qValues[idx] > maxQ {
			maxQ = qValues[idx]
			bestAction = idx
		}
	}

	return bestAction
}

// Learn updates the Q-Table based on the transition.
func (a *AgentQTable) Learn(state State, action int, reward float64, nextState State) {
	// Get current Q
	qValues := a.QTable[state]
	currentQ := qValues[action]

	// Get max Q for next state
	nextQValues, exists := a.QTable[nextState]
	maxNextQ := 0.0
	if exists {
		maxNextQ = -math.MaxFloat64
		for _, q := range nextQValues {
			if q > maxNextQ {
				maxNextQ = q
			}
		}
	}

	// Bellman Equation
	// Q(s,a) = Q(s,a) + Alpha * (R + Gamma * maxQ(s',a') - Q(s,a))
	newQ := currentQ + Alpha*(reward+Gamma*maxNextQ-currentQ)

	qValues[action] = newQ
	a.QTable[state] = qValues
}

func (a *AgentQTable) DebugInfoStr() string {
	return fmt.Sprintf("Agent Type: Q-Table\nQ-Table Size: %d", len(a.QTable))
}

// CalculateReward determines the reward for the current state.
func CalculateReward(c *physics.Car, grid *track.Grid, mesh *track.TrackMesh) float64 {
	if c.Crashed {
		return -100.0
	}

	// 1. Progress Reward
	// We want to maximize speed along the track direction (s-velocity)
	wp, _ := mesh.GetClosestWaypoint(c.Position)

	// Tangent vector
	tangentX := wp.Normal.Y
	tangentY := -wp.Normal.X

	// Dot product of Velocity and Tangent = Speed along track
	speedAlongTrack := c.Velocity.X*tangentX + c.Velocity.Y*tangentY

	reward := speedAlongTrack * 2.0 // Multiplier to encourage speed

	// TODO: see if rewards can be issued for being at the right places in corners / turns - close to the outside edge of the road during corner entry and inside while hitting the apex, then close to the outside again when meeting the next section of the road (roughly).
	// also see if rewards can be provided for optimum brake / throttle / accel levels during corner entry and exit.

	// DISABLED : AI suggested centering reward 
	// // 2. Centering Reward (Stay in middle lanes)
	// // Calculate Lateral Offset (d)
	// dx := c.Position.X - wp.Position.X
	// dy := c.Position.Y - wp.Position.Y
	// d := dx*wp.Normal.X + dy*wp.Normal.Y

	// if math.Abs(d) > 20 {
	// 	reward -= 2.0 // Penalty for being near edge
	// }

	// 3. Gravel Penalty
	cellX := int(c.Position.X)
	cellY := int(c.Position.Y)
	cell := grid.Get(cellX, cellY)

	if cell.Type == track.CellGravel {
		reward -= 5.0
	}

	return reward
}
