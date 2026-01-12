package track

import "image/color"

// CellType represents the type of surface in a grid cell.
type CellType int

const (
	CellWall CellType = iota
	CellTarmac
	CellGravel
	CellStart
	CellFinish
	CellDirection // For manual heading hint
)

// Cell represents a single unit of the track.
type Cell struct {
	Type     CellType
	Friction float64 // 1.0 for Tarmac, 0.5 for Gravel, etc.
}

// Grid represents the discretized track.
type Grid struct {
	Width, Height int
	Cells         [][]Cell
	Scale         float64 // Meters per pixel/cell
}

// NewGrid creates a new grid of the specified size.
func NewGrid(width, height int) *Grid {
	cells := make([][]Cell, width)
	for i := range cells {
		cells[i] = make([]Cell, height)
	}
	return &Grid{
		Width:  width,
		Height: height,
		Cells:  cells,
		Scale:  1.0, // Default 1 meter per cell
	}
}

// Get returns the cell at (x, y). Returns Wall if out of bounds.
func (g *Grid) Get(x, y int) Cell {
	if x < 0 || x >= g.Width || y < 0 || y >= g.Height {
		return Cell{Type: CellWall, Friction: 0.0}
	}
	return g.Cells[x][y]
}

// ColorToCellType maps a pixel color to a cell type.
// This is a simple threshold-based mapper.
func ColorToCellType(c color.Color) CellType {
	r, g, b, _ := c.RGBA()
	// Normalize to 8-bit
	r8, g8, b8 := r>>8, g>>8, b>>8

	// White/Light Gray = Tarmac
	if r8 > 200 && g8 > 200 && b8 > 200 {
		return CellTarmac
	}
	// Red = Start/Finish
	if r8 > 200 && g8 < 100 && b8 < 100 {
		return CellStart
	}
	// Yellow = Direction Hint
	if r8 > 200 && g8 > 200 && b8 < 100 {
		return CellDirection
	}
	// Green = Gravel
	if g8 > r8+50 && g8 > b8+50 {
		return CellGravel
	}

	// Default Fallback logic:
	// If it's Dark, it's a Wall.
	// If it's Bright (but didn't match above), it's likely an anti-aliased edge of a marker or track. Treat as Tarmac.
	if r8 < 50 && g8 < 50 && b8 < 50 {
		return CellWall
	}

	return CellTarmac
}
