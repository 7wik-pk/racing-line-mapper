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
	Alpha float64     = 0.1   // Learning Rate
	Gamma float64      = 0.999987 // Discount Factor
	MinEpsilon float64 = 0.005
	Decay      float64 = 0.9999875 // Decay Rate
)

var Epsilon = 1.0

// Rewards
const (
	RwCrash                     = -100.0
	RwSpeedAlongTrackMultiplier = 1.0
	RwGravel                    = -5.0
)

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

	Epsilon = math.Max(Epsilon*Decay, MinEpsilon)

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
	return fmt.Sprintf("Type: Q-Table\nQ-Size:  %d\nAlpha:   %.8f\nGamma:   %.8f\nEpsilon: %.8f\nDecay:   %.8f",
		len(a.QTable), Alpha, Gamma, Epsilon, Decay)
}

// CalculateReward determines the reward for the current state.
func CalculateReward(c *physics.Car, grid *track.Grid, mesh *track.TrackMesh, bestLapTime int) float64 {
	if c.Crashed {
		return RwCrash
	}

	// 1. Progress Reward
	// We want to maximize speed along the track direction (s-velocity)
	wp, wpIdx := mesh.GetClosestWaypoint(c.Position)

	// Tangent vector
	tangentX := wp.Normal.Y
	tangentY := -wp.Normal.X

	// Dot product of Velocity and Tangent = Speed along track
	speedAlongTrack := c.Velocity.X*tangentX + c.Velocity.Y*tangentY

	reward := speedAlongTrack * RwSpeedAlongTrackMultiplier // Multiplier to encourage speed

	// TODO: see if rewards can be issued for being at the right places in corners / turns - close to the outside edge of the road during corner entry and inside while hitting the apex, then close to the outside again when meeting the next section of the road (roughly).
	// also see if rewards can be provided for optimum brake / throttle / accel levels during corner entry and exit.

	// 2. Centering Reward (Stay in middle lanes)
	// Calculate Lateral Offset (d)
	dx := c.Position.X - wp.Position.X
	dy := c.Position.Y - wp.Position.Y
	d := dx*wp.Normal.X + dy*wp.Normal.Y

	if math.Abs(d) > 20 {
		reward -= 2.0 // Penalty for being near edge
	}

	// 3. Gravel Penalty
	cellX := int(c.Position.X)
	cellY := int(c.Position.Y)
	cell := grid.Get(cellX, cellY)

	if cell.Type == track.CellGravel {
		reward -= RwGravel
	}

	// 4. Time/Stationary Penalty
	// Penalize just existing to encourage finishing fast
	// Extra penalty if actually stopped
	reward -= 1.0

	if c.Speed < 0.1 {
		reward -= 10.0 // Heavy penalty for stopping
	}

	// 5. Backwards Penalty
	// If speedAlongTrack is negative, we are going wrong way
	if speedAlongTrack < -0.1 {
		reward -= 20.0 // Very heavy penalty for wrong way
	}

	// 6. Checkpoint & Lap Reward

	// Check strictly sequential progress
	// Allow small skips (e.g. 1->3 is ok, 1->10 is cheating/cutting)
	// Also handle lap wrap-around (End -> 0)

	validProgress := false
	diff := wpIdx - c.Checkpoint

	// Normal process: moved forward by 1-5 waypoints
	if diff > 0 && diff < 10 {
		validProgress = true
	}

	// Lap wrap-around: Last few checkpoints -> First few
	// e.g. MeshLen=100. Current=98. Next=1.
	if c.Checkpoint > len(mesh.Waypoints)-10 && wpIdx < 10 {
		validProgress = true
		c.Laps++

		// Major Lap Reward base
		reward += 1000.0

		// Personal Best Bonus
		// If we beat the best time (or if no best time exists/0), give bonus
		// bestLapTime comes from Game, in ticks.
		// c.CurrentLapTime is what we just finished.

		// Note: c.CurrentLapTime is handled in main loop tick update, let's assume it's accurate at moment of crossing.
		if bestLapTime > 0 && c.CurrentLapTime < bestLapTime {
			// Improvement Bonus
			improvement := float64(bestLapTime - c.CurrentLapTime)
			// e.g. Improved by 100 ticks (1.6s) -> 100 * 5 = 500 extra reward
			reward += improvement * 5.0

			// Just for beating PB
			reward += 500.0
		}
	}

	if validProgress || c.Checkpoint == -1 {
		c.Checkpoint = wpIdx
		// Small bonus for verifying checkpoint (milestone)
		reward += 10.0
	}

	return reward
}
