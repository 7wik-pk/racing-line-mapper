package track

import (
	"math"
	"racing-line-mapper/internal/common"
)

// Waypoint represents a point on the track centerline.
type Waypoint struct {
	ID       int
	Position common.Vec2 // World coordinates (x, y)
	Normal   common.Vec2 // Unit vector perpendicular to the track direction (pointing Right)
	Width    float64     // Width of the track at this point
	Distance float64     // Distance from start (s-coordinate)
}

// TrackMesh represents the curvilinear coordinate system of the track.
type TrackMesh struct {
	Waypoints []Waypoint
	TotalLen  float64
}

// GetClosestWaypoint finds the waypoint closest to the given world position.
// Returns the waypoint and its index.
// TODO Optimization: In a real app, use a spatial hash or quadtree. Here, linear search is fine for < 1000 points.
func (m *TrackMesh) GetClosestWaypoint(pos common.Vec2) (Waypoint, int) {
	minDistSq := math.MaxFloat64
	closestIdx := -1

	for i, wp := range m.Waypoints {
		dx := pos.X - wp.Position.X
		dy := pos.Y - wp.Position.Y
		distSq := dx*dx + dy*dy
		if distSq < minDistSq {
			minDistSq = distSq
			closestIdx = i
		}
	}

	if closestIdx == -1 {
		return Waypoint{}, -1
	}
	return m.Waypoints[closestIdx], closestIdx
}

// WorldToFrenet converts World (x,y) to Frenet (s,d).
// s: Progress along track
// d: Lateral offset (positive = right of center, negative = left)
func (m *TrackMesh) WorldToFrenet(pos common.Vec2) (float64, float64) {
	wp, _ := m.GetClosestWaypoint(pos)

	// Vector from Waypoint to Pos
	dx := pos.X - wp.Position.X
	dy := pos.Y - wp.Position.Y

	// Project onto Normal to get 'd' (Lateral offset)
	// Normal is unit vector. Dot product gives scalar projection.
	d := dx*wp.Normal.X + dy*wp.Normal.Y

	// 's' is roughly the waypoint's distance.
	// For more precision, we'd project onto the tangent and add that small delta.
	// But for discrete RL, waypoint distance is sufficient.
	s := wp.Distance

	return s, d
}
