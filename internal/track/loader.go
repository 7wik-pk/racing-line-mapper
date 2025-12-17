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
func GenerateMesh(grid *Grid, startX, startY int) *TrackMesh {
	rawWaypoints := []Waypoint{}

	// 1. Find Center of Track at Start
	leftX := startX
	for leftX > 0 && grid.Cells[leftX][startY].Type != CellWall {
		leftX--
	}
	rightX := startX
	for rightX < grid.Width-1 && grid.Cells[rightX][startY].Type != CellWall {
		rightX++
	}

	centerX := float64(leftX+rightX) / 2.0
	centerY := float64(startY)
	trackWidth := float64(rightX - leftX)

	currX, currY := centerX, centerY

	// Initial Direction (Assume East for start)
	dirX, dirY := 1.0, 0.0

	stepSize := 20.0
	visited := make(map[int]bool)

	for i := 0; i < 2000; i++ {
		// Just move forward a bit to start the raycast
		// Raycast in an arc to find the "deepest" path
		bestAngle := 0.0
		maxDepth := 0.0

		baseAngle := math.Atan2(dirY, dirX)

		// Scan a wider arc to handle sharp turns
		for angle := -math.Pi / 2; angle <= math.Pi/2; angle += math.Pi / 32 {
			checkAngle := baseAngle + angle
			dx := math.Cos(checkAngle)
			dy := math.Sin(checkAngle)

			// Beam length
			depth := 0.0
			for d := 5.0; d < 150.0; d += 5.0 {
				cx := int(currX + dx*d)
				cy := int(currY + dy*d)

				// Stop at wall OR if checking backwards (visited)
				// Simple visited check: don't go back near start unless looping
				if grid.Get(cx, cy).Type == CellWall {
					break
				}
				depth = d
			}

			// Bias towards straight lines slightly to reduce jitter
			// depth *= (1.0 - math.Abs(angle)*0.1)

			if depth > maxDepth {
				maxDepth = depth
				bestAngle = checkAngle
			}
		}

		// New Direction
		newDirX := math.Cos(bestAngle)
		newDirY := math.Sin(bestAngle)

		// Move to new point
		currX += newDirX * stepSize
		currY += newDirY * stepSize

		// Update persistent direction (Exponential Moving Average for smoothness)
		dirX = dirX*0.2 + newDirX*0.8
		dirY = dirY*0.2 + newDirY*0.8

		// Save Waypoint
		// Normal is purely based on movement direction for now (Perpendicular)
		normX, normY := -dirY, dirX
		l := math.Sqrt(normX*normX + normY*normY)

		wp := Waypoint{
			ID:       i,
			Position: common.Vec2{X: currX, Y: currY},
			Normal:   common.Vec2{X: normX / l, Y: normY / l},
			Width:    trackWidth,
			Distance: float64(i) * stepSize,
		}
		rawWaypoints = append(rawWaypoints, wp)

		visited[int(currY)*grid.Width+int(currX)] = true

		// Loop Closure Check
		// Only check if we have traveled a bit
		if i > 50 {
			distToStart := math.Sqrt((currX-centerX)*(currX-centerX) + (currY-centerY)*(currY-centerY))
			if distToStart < stepSize*2 {
				break
			}
		}
	}

	// 2. Refinement Pass ("Elastic Band" / Iterative Centering)
	// The initial walker might be biased or cut corners.
	// We iterate to pull every point towards the true geometric center.
	refinedWaypoints := make([]Waypoint, len(rawWaypoints))
	copy(refinedWaypoints, rawWaypoints)

	// Number of relaxation iterations
	for iter := 0; iter < 10; iter++ {
		for i := 0; i < len(refinedWaypoints); i++ {
			wp := refinedWaypoints[i]

			// Calculate approximate tangent from neighbors
			prev := refinedWaypoints[(i-1+len(refinedWaypoints))%len(refinedWaypoints)]
			next := refinedWaypoints[(i+1)%len(refinedWaypoints)]

			tx := next.Position.X - prev.Position.X
			ty := next.Position.Y - prev.Position.Y

			// Normal = (-ty, tx)
			nx, ny := -ty, tx
			l := math.Sqrt(nx*nx + ny*ny)
			if l == 0 {
				continue
			}
			nx /= l
			ny /= l

			// Raycast Left/Right to find walls
			dLeft := 0.0
			foundLeft := false
			for d := 1.0; d < 80.0; d += 1.0 {
				cx := int(wp.Position.X + nx*d)
				cy := int(wp.Position.Y + ny*d)
				if grid.Get(cx, cy).Type == CellWall {
					dLeft = d
					foundLeft = true
					break
				}
			}

			dRight := 0.0
			foundRight := false
			for d := 1.0; d < 80.0; d += 1.0 {
				cx := int(wp.Position.X - nx*d)
				cy := int(wp.Position.Y - ny*d)
				if grid.Get(cx, cy).Type == CellWall {
					dRight = d
					foundRight = true
					break
				}
			}

			// Move point towards center
			if foundLeft && foundRight {
				// We want dLeft == dRight.
				// Error = dLeft - dRight.
				// Correction = Error / 2
				correction := (dLeft - dRight) / 2.0

				// Alpha blend for stability (0.5)
				refinedWaypoints[i].Position.X += nx * correction * 0.5
				refinedWaypoints[i].Position.Y += ny * correction * 0.5

				// Update Width estimate
				refinedWaypoints[i].Width = dLeft + dRight
			}
		}
	}

	// 3. Final Smoothing Pass (Moving Average)
	smoothedWaypoints := make([]Waypoint, len(refinedWaypoints))
	copy(smoothedWaypoints, refinedWaypoints)

	// Two passes of smoothing for positions
	for pass := 0; pass < 2; pass++ {
		temp := make([]Waypoint, len(smoothedWaypoints))
		copy(temp, smoothedWaypoints)

		for i := 0; i < len(smoothedWaypoints); i++ {
			sumX, sumY := 0.0, 0.0
			window := 5
			for j := -window / 2; j <= window/2; j++ {
				idx := (i + j + len(smoothedWaypoints)) % len(smoothedWaypoints)
				sumX += temp[idx].Position.X
				sumY += temp[idx].Position.Y
			}
			smoothedWaypoints[i].Position.X = sumX / float64(window)
			smoothedWaypoints[i].Position.Y = sumY / float64(window)
		}
	}

	// Recompute Final Normals
	for i := 0; i < len(smoothedWaypoints); i++ {
		prev := smoothedWaypoints[(i-1+len(smoothedWaypoints))%len(smoothedWaypoints)]
		next := smoothedWaypoints[(i+1)%len(smoothedWaypoints)]

		dx := next.Position.X - prev.Position.X
		dy := next.Position.Y - prev.Position.Y

		nx, ny := -dy, dx
		len := math.Sqrt(nx*nx + ny*ny)
		if len > 0 {
			smoothedWaypoints[i].Normal = common.Vec2{X: nx / len, Y: ny / len}
		}

		// Copy width from refined
		smoothedWaypoints[i].Width = refinedWaypoints[i].Width
	}

	return &TrackMesh{
		Waypoints: smoothedWaypoints,
		TotalLen:  float64(len(smoothedWaypoints)) * stepSize,
	}
}
