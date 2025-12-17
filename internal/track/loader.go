package track

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"racing-line-mapper/internal/common"
)

// LoadTrackFromImage loads an image and converts it to a Grid.
func LoadTrackFromImage(path string) (*Grid, *TrackMesh, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, nil, err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y
	grid := NewGrid(width, height)

	// Keep track of tarmac pixels for finding start point
	var startX, startY int
	foundStart := false

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			c := img.At(x, y)
			cellType := ColorToCellType(c)

			friction := 1.0
			switch cellType {
			case CellGravel:
				friction = 0.4
			case CellWall:
				friction = 0.0
			}

			grid.Cells[x][y] = Cell{
				Type:     cellType,
				Friction: friction,
			}

			if cellType == CellStart {
				startX, startY = x, y
				foundStart = true
			}
		}
	}

	// If no explicit start, find first tarmac
	if !foundStart {
		// Simple search
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if grid.Cells[x][y].Type == CellTarmac {
					startX, startY = x, y
					foundStart = true
					break
				}
			}
			if foundStart {
				break
			}
		}
	}

	mesh := GenerateMesh(grid, startX, startY)

	return grid, mesh, nil
}

// GenerateMesh creates a centerline mesh from the grid.
// This is a simplified "Flood Fill + Averaging" approach.
// A robust approach would use Distance Transform + Skeletonization.
// For this demo, we will trace the track by "walking" along the tarmac.
func GenerateMesh(grid *Grid, startX, startY int) *TrackMesh {
	waypoints := []Waypoint{}

	// 1. Find Center of Track at Start
	// Scan left/right to find edges
	leftX := startX
	for leftX > 0 && grid.Cells[leftX][startY].Type != CellWall {
		leftX--
	}
	rightX := startX
	for rightX < grid.Width-1 && grid.Cells[rightX][startY].Type != CellWall {
		rightX++
	}

	centerX := (leftX + rightX) / 2
	centerY := startY
	trackWidth := float64(rightX - leftX)

	// 2. Walk the track
	// We need a direction. Assume Start Line is vertical, so we move "Forward" (e.g. +X or +Y depending on track).
	// Let's assume Counter-Clockwise loop.
	// We'll use a "Walker" that looks ahead.

	currX, currY := float64(centerX), float64(centerY)
	visited := make(map[int]bool) // Key: y*width + x

	// Initial direction: Try to find open space
	dirX, dirY := 1.0, 0.0 // Try East first

	// Refined Walker:
	// At each step, cast rays in an arc to find the "furthest open tarmac".
	// This is like a "Lidar" based path finding.

	totalDist := 0.0

	for i := 0; i < 1000; i++ { // Safety break
		// Record Waypoint
		// Calculate Normal (Perpendicular to Dir)
		normX, normY := -dirY, dirX // Rotate 90 deg

		wp := Waypoint{
			ID:       i,
			Position: common.Vec2{X: currX, Y: currY},
			Normal:   common.Vec2{X: normX, Y: normY}.Normalize(),
			Width:    trackWidth, // Approximation
			Distance: totalDist,
		}
		waypoints = append(waypoints, wp)

		// Mark visited (roughly)
		idx := int(currY)*grid.Width + int(currX)
		visited[idx] = true

		// Look ahead to find next center
		// Cast 5 rays: -45, -22.5, 0, +22.5, +45 degrees relative to current Dir
		bestAngle := 0.0
		maxDepth := 0.0

		baseAngle := math.Atan2(dirY, dirX)

		for angleOffset := -math.Pi / 3; angleOffset <= math.Pi/3; angleOffset += math.Pi / 8 {
			checkAngle := baseAngle + angleOffset
			dx := math.Cos(checkAngle)
			dy := math.Sin(checkAngle)

			// Raycast
			depth := 0.0
			for d := 1.0; d < 100.0; d += 2.0 {
				checkX := int(currX + dx*d)
				checkY := int(currY + dy*d)

				if grid.Get(checkX, checkY).Type == CellWall {
					break
				}

				// Don't go back immediately (simple check)
				// if visited[checkY*grid.Width+checkX] { depth -= 5.0 }

				depth = d
			}

			if depth > maxDepth {
				maxDepth = depth
				bestAngle = checkAngle
			}
		}

		// Move
		stepSize := 20.0 // Distance between waypoints
		dirX = math.Cos(bestAngle)
		dirY = math.Sin(bestAngle)

		nextX := currX + dirX*stepSize
		nextY := currY + dirY*stepSize

		// Check if we looped (close to start)
		distToStart := math.Sqrt((nextX-float64(centerX))*(nextX-float64(centerX)) + (nextY-float64(centerY))*(nextY-float64(centerY)))
		if i > 10 && distToStart < stepSize*1.5 {
			break // Loop closed
		}

		currX = nextX
		currY = nextY
		totalDist += stepSize
	}

	return &TrackMesh{
		Waypoints: waypoints,
		TotalLen:  totalDist,
	}
}
