package track

import (
	"fmt"
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

	// Keep track of start pixels to find centroid
	var startXSum, startYSum, startCount int

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
				startXSum += x
				startYSum += y
				startCount++
			}
		}
	}

	var startX, startY int
	foundStart := false
	if startCount > 0 {
		startX = startXSum / startCount
		startY = startYSum / startCount
		foundStart = true
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

	// 1. Determine Start Direction
	// Priority: Use Yellow Marker (CellDirection) if present.

	var dirX, dirY float64

	// Find Yellow Centroid
	var yellowXSum, yellowYSum, yellowCount int
	for x := 0; x < grid.Width; x++ {
		for y := 0; y < grid.Height; y++ {
			if grid.Cells[x][y].Type == CellDirection {
				yellowXSum += x
				yellowYSum += y
				yellowCount++
			}
		}
	}

	if yellowCount > 0 {
		yellowX := float64(yellowXSum) / float64(yellowCount)
		yellowY := float64(yellowYSum) / float64(yellowCount)

		dx := yellowX - float64(startX)
		dy := yellowY - float64(startY)
		l := math.Sqrt(dx*dx + dy*dy)
		if l > 0 {
			dirX, dirY = dx/l, dy/l
		} else {
			dirX, dirY = 1.0, 0.0 // Start and Direction are same point? Default East
		}
		fmt.Printf("Use Yellow Heading: Start(%.1f, %.1f) -> Yellow(%.1f, %.1f) | Dir(%.2f, %.2f)\n",
			float64(startX), float64(startY), yellowX, yellowY, dirX, dirY)
	} else {
		// Fallback: Default to East (1,0) as requested to remove 360 scan
		dirX, dirY = 1.0, 0.0
		fmt.Printf("No Yellow Marker found. Defaulting to East (1.0, 0.0). Start(%.1f, %.1f)\n", float64(startX), float64(startY))
	}

	// 2. Find True Center & Width relative to Direction
	// Scan perpendicular to direction (Normal)
	normX, normY := -dirY, dirX

	// Find borders
	leftDist, rightDist := 0.0, 0.0
	for k := 0.0; k < 100.0; k += 1.0 {
		if grid.Get(int(float64(startX)+normX*k), int(float64(startY)+normY*k)).Type == CellWall {
			leftDist = k
			break
		}
	}
	for k := 0.0; k < 100.0; k += 1.0 {
		if grid.Get(int(float64(startX)-normX*k), int(float64(startY)-normY*k)).Type == CellWall {
			rightDist = k
			break
		}
	}

	trackWidth := leftDist + rightDist
	if trackWidth < 2 {
		trackWidth = 20
	}

	// Center is startPos shifted by (left - right)/2 ? No.
	// Start is at 0 relative to scan. Left wall at +L. Right wall at -R.
	// Width = L+R. Midpoint is at (L-R)/2 from Start.
	centerOffset := (leftDist - rightDist) / 2.0
	centerX := float64(startX) + normX*centerOffset
	centerY := float64(startY) + normY*centerOffset

	currX, currY := centerX, centerY
	totalDist := 0.0

	stepSize := 6.0 // "Sweet spot" attempt (not 4, not 8)
	visited := make(map[int]bool)

	for i := 0; i < 6000; i++ {
		// Scan an arc to find the "deepest" path
		bestAngle := 0.0
		maxDepth := -999.0
		baseAngle := math.Atan2(dirY, dirX)

		// Search in a 120-degree arc with high resolution
		for angle := -math.Pi / 1.5; angle <= math.Pi/1.5; angle += math.Pi / 64 {
			checkAngle := baseAngle + angle
			dx := math.Cos(checkAngle)
			dy := math.Sin(checkAngle)

			depth := 0.0
			foundVisited := false
			// Max depth to avoid bridging hairpins
			for d := 2.0; d < 100.0; d += 2.0 {
				cx, cy := int(currX+dx*d), int(currY+dy*d)
				if grid.Get(cx, cy).Type == CellWall {
					break
				}
				if visited[cy*grid.Width+cx] {
					foundVisited = true
				}
				depth = d
			}

			// Turning penalty
			score := depth * (1.1 - math.Abs(angle)/math.Pi)
			if foundVisited {
				score *= 0.1 // Heavy penalty for going back
			}

			if score > maxDepth {
				maxDepth = score
				bestAngle = checkAngle
			}
		}

		newDirX := math.Cos(bestAngle)
		newDirY := math.Sin(bestAngle)

		// Move
		prevX, prevY := currX, currY
		currX += newDirX * stepSize
		currY += newDirY * stepSize

		// Smooth direction
		dirX = dirX*0.4 + newDirX*0.6
		dirY = dirY*0.4 + newDirY*0.6

		// Update Distance
		actualStep := math.Sqrt(math.Pow(currX-prevX, 2) + math.Pow(currY-prevY, 2))
		totalDist += actualStep

		// Mark as visited (with small footprint to guide the tracer)
		for vx := -2; vx <= 2; vx++ {
			for vy := -2; vy <= 2; vy++ {
				vx_off, vy_off := int(currX)+vx, int(currY)+vy
				if vx_off >= 0 && vx_off < grid.Width && vy_off >= 0 && vy_off < grid.Height {
					visited[vy_off*grid.Width+vx_off] = true
				}
			}
		}

		// Save Waypoint
		normX, normY := -dirY, dirX
		l := math.Sqrt(normX*normX + normY*normY)

		wp := Waypoint{
			ID:       i,
			Position: common.Vec2{X: currX, Y: currY},
			Normal:   common.Vec2{X: normX / l, Y: normY / l},
			Width:    trackWidth,
			Distance: totalDist,
		}
		rawWaypoints = append(rawWaypoints, wp)

		// Loop Closure Check (After traveling enough)
		if i > 150 {
			distToStart := math.Sqrt((currX-centerX)*(currX-centerX) + (currY-centerY)*(currY-centerY))
			if distToStart < stepSize*2.0 {
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

	// Two passes of smoothing for positions with larger window
	for pass := 0; pass < 2; pass++ {
		temp := make([]Waypoint, len(smoothedWaypoints))
		copy(temp, smoothedWaypoints)

		for i := 0; i < len(smoothedWaypoints); i++ {
			sumX, sumY := 0.0, 0.0
			window := 3 // Reduced from 15 to 3 to preserve curve geometry
			for j := -window / 2; j <= window/2; j++ {
				idx := (i + j + len(smoothedWaypoints)) % len(smoothedWaypoints)
				sumX += temp[idx].Position.X
				sumY += temp[idx].Position.Y
			}
			smoothedWaypoints[i].Position.X = sumX / float64(window)
			smoothedWaypoints[i].Position.Y = sumY / float64(window)
		}
	}

	// Recompute Final Normals with explicit normal smoothing
	for i := 0; i < len(smoothedWaypoints); i++ {
		// Calculate Raw Normal from smoothed positions
		prev := smoothedWaypoints[(i-1+len(smoothedWaypoints))%len(smoothedWaypoints)]
		next := smoothedWaypoints[(i+1)%len(smoothedWaypoints)]

		dx := next.Position.X - prev.Position.X
		dy := next.Position.Y - prev.Position.Y

		// Raw Normal
		nx, ny := -dy, dx
		len := math.Sqrt(nx*nx + ny*ny)
		if len > 0 {
			smoothedWaypoints[i].Normal = common.Vec2{X: nx / len, Y: ny / len}
		}

		smoothedWaypoints[i].Width = refinedWaypoints[i].Width
	}

	// Explicit Normal Smoothing Pass
	finalMeshPoints := make([]Waypoint, len(smoothedWaypoints))
	copy(finalMeshPoints, smoothedWaypoints)

	for pass := 0; pass < 2; pass++ { // 2 passes of normal smoothing
		temp := make([]Waypoint, len(finalMeshPoints))
		copy(temp, finalMeshPoints)

		for i := 0; i < len(finalMeshPoints); i++ {
			sumNx, sumNy := 0.0, 0.0
			window := 5
			for j := -window / 2; j <= window/2; j++ {
				idx := (i + j + len(finalMeshPoints)) % len(finalMeshPoints)
				sumNx += temp[idx].Normal.X
				sumNy += temp[idx].Normal.Y
			}
			// Normalize averaged normal
			l := math.Sqrt(sumNx*sumNx + sumNy*sumNy)
			if l > 0 {
				finalMeshPoints[i].Normal.X = sumNx / l
				finalMeshPoints[i].Normal.Y = sumNy / l
			}
		}
	}

	smoothedWaypoints = finalMeshPoints

	return &TrackMesh{
		Waypoints: smoothedWaypoints,
		TotalLen:  float64(len(smoothedWaypoints)) * stepSize,
	}
}
