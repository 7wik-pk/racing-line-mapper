package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	width, height := 800, 600
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with White (Wall)
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, white)
		}
	}

	// Draw Tarmac (Black) - A simple oval
	black := color.RGBA{0, 0, 0, 255}
	centerX, centerY := width/2, height/2
	radiusX, radiusY := 300.0, 200.0
	trackWidth := 50.0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx := float64(x - centerX)
			dy := float64(y - centerY)

			// Ellipse equation: (x/a)^2 + (y/b)^2 = 1
			dist := (dx*dx)/(radiusX*radiusX) + (dy*dy)/(radiusY*radiusY)

			// If inside the outer edge and outside the inner edge
			if dist <= 1.0 && dist >= 0.6 {
				img.Set(x, y, black)
			}
		}
	}

	// Draw Start Line (Red)
	red := color.RGBA{255, 0, 0, 255}
	for y := centerY - int(radiusY); y < centerY-int(radiusY)+int(trackWidth); y++ {
		for x := centerX - 10; x < centerX+10; x++ {
			// Check if it's on tarmac before drawing
			if img.RGBAAt(x, y) == black {
				img.Set(x, y, red)
			}
		}
	}

	// Draw Gravel (Green) - Just a patch outside the turn
	green := color.RGBA{0, 255, 0, 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, green)
		}
	}

	f, _ := os.Create("assets/track.png")
	defer f.Close()
	png.Encode(f, img)
}
