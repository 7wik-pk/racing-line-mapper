package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

func main() {

	fmt.Println("running debug preproc script...")

	// Load the image
	input_path := "./input_track_maps/monza_10m.jpg"
	img := gocv.IMRead(input_path, gocv.IMReadColor)
	if img.Empty() {
		fmt.Printf("Error reading image\n")
		return
	}
	defer img.Close()

	// extract the original filename after slashes
	outputFilename := filepath.Base(input_path)

	// convert to grayscale and invert black and white
	gocv.CvtColor(img, &img, gocv.ColorBGRToGray)
	gocv.BitwiseNot(img, &img)

	// Padding to remove white bleeding into the edges of the image when morphologically closing
	// Define padding amounts for all sides
	top := 64
	bottom := 64
	left := 64
	right := 64

	// Define the border color (black for zero padding)
	// Make sure the Scalar matches the image's number of channels (e.g., 3 for color images)
	black := color.RGBA{0, 0, 0, 0}

	// The type of the destination Mat should match the source Mat
	gocv.CopyMakeBorder(img, &img, top, bottom, left, right, gocv.BorderConstant, black)

	var output gocv.Mat
	output = img
	defer output.Close()

	output = Threshold(output, 150, 255)
	// kernel sizes that work best for all tracks: 
	// monza - 13
	// spa - 6
	output = Open(output, 13, 1)

	// close gaps with skeletonization + connecting endpoints
	thin := ThinTrack(output)
	defer thin.Close()
	thin = CloseGapsByEndpoints(thin)

	// widen the thin track to original width
	output = RestoreUniformThickness(output, thin)

	// Save the result
	if ok := gocv.IMWrite("./processed_tracks/" + outputFilename, output); !ok {
		fmt.Printf("Error writing image\n")
	}
	fmt.Println("Output saved to ./processed_tracks/" + outputFilename)
}

func Canny(img gocv.Mat) gocv.Mat {
	// Convert to grayscale
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(img, &gray, gocv.ColorBGRToGray)

	// Apply Canny edge detection
	// Define thresholds (these often require tuning for different images)
	// Weak edges connected to strong edges are kept (hysteresis thresholding)
	lowThreshold := float32(100)
	highThreshold := float32(250)
	cannyOutput := gocv.NewMat()
	// Do NOT defer Close() here if we are returning it!
	// The caller is responsible for closing the returned Mat.

	gocv.Canny(gray, &cannyOutput, lowThreshold, highThreshold)

	fmt.Println("Canny edge detection applied to input img")
	return cannyOutput
}

func Erode(img gocv.Mat, kernelSize int, iterations int) gocv.Mat {
	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(kernelSize, kernelSize))
	output := gocv.NewMat()
	
	for i := 0; i < iterations; i++ {
		gocv.Erode(img, &output, kernel)
		img = output
	}

	return output
}

func Threshold(img gocv.Mat, min int, max int) gocv.Mat {

	output := gocv.NewMat()
	gocv.Threshold(img, &output, float32(min), float32(max), gocv.ThresholdBinary)

	return output
}

func Close(img gocv.Mat, kernelSize int, iterations int) gocv.Mat {
	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(kernelSize, kernelSize))
	output := gocv.NewMat()

	for i := 0; i < iterations; i++ {
		gocv.MorphologyEx(img, &output, gocv.MorphClose, kernel)
		img = output
	}
	return output
}

func Open(img gocv.Mat, kernelSize int, iterations int) gocv.Mat {
	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(kernelSize, kernelSize))
	output := gocv.NewMat()

	for i := 0; i < iterations; i++ {
		gocv.MorphologyEx(img, &output, gocv.MorphOpen, kernel)
		img = output
	}

	return output
}

func Dilate(img gocv.Mat, kernelSize int, iterations int) gocv.Mat {
	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(kernelSize, kernelSize))
	output := gocv.NewMat()
	
	for i := 0; i < iterations; i++ {
		gocv.Dilate(img, &output, kernel)
		img = output
	}
	return output
}

func ThinTrack(src gocv.Mat) gocv.Mat {
    dst := gocv.NewMat()
    // ThinningTypes can be ThinningZhangSuen or ThinningGuoHall
    contrib.Thinning(src, &dst, contrib.ThinningZhangSuen)
    return dst
}

// Curve fitting - to close the gaps (in monza)

func CloseGapsByEndpoints(img gocv.Mat) gocv.Mat {
	// 1. Get all contours
	contours := gocv.FindContours(img, gocv.RetrievalExternal, gocv.ChainApproxNone)
	defer contours.Close()

	type Tip struct {
		Point     image.Point
		ContourID int
	}
	var allTips []Tip

	// 2. Iterate through each contour to find "True Tips"
	for i := 0; i < contours.Size(); i++ {
		pts := contours.At(i).ToPoints()
		
		for _, p := range pts {
			if isEndpoint(img, p.X, p.Y) {
				allTips = append(allTips, Tip{Point: p, ContourID: i})
			}
		}
	}

	// 3. Connect nearest endpoints from DIFFERENT contours
	result := img.Clone()
	maxGap := 100.0

	for i := 0; i < len(allTips); i++ {
		bestDist := maxGap
		bestMatchIdx := -1

		for j := 0; j < len(allTips); j++ {
			// Only connect to a different contour
			if allTips[i].ContourID == allTips[j].ContourID {
				continue
			}

			dx := float64(allTips[i].Point.X - allTips[j].Point.X)
			dy := float64(allTips[i].Point.Y - allTips[j].Point.Y)
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist < bestDist {
				bestDist = dist
				bestMatchIdx = j
			}
		}

		if bestMatchIdx != -1 {
			gocv.Line(&result, allTips[i].Point, allTips[bestMatchIdx].Point, color.RGBA{255, 255, 255, 0}, 1)
		}
	}

	return result
}

// isEndpoint checks the 3x3 neighborhood of a pixel.
// A pixel is a tip if it has 1 to 3 neighbors (excluding itself).
func isEndpoint(img gocv.Mat, x, y int) bool {
	neighborCount := 0
	
	// Boundary checks for the 3x3 window
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue // Skip the pixel itself
			}
			
			currX, currY := x+i, y+j
			if currX >= 0 && currY >= 0 && currX < img.Cols() && currY < img.Rows() {
				if img.GetUCharAt(currY, currX) > 0 {
					neighborCount++
				}
			}
		}
	}
	
	// A "true" dead end in a 1px line has 1 neighbor.
	return neighborCount == 1
}

func RestoreUniformThickness(thickTrack gocv.Mat, skeleton gocv.Mat) gocv.Mat {
    distMap := gocv.NewMat()
    defer distMap.Close()
    labels := gocv.NewMat()
    defer labels.Close()

    // 1. Calculate the map
    gocv.DistanceTransform(thickTrack, &distMap, &labels, gocv.DistL2, gocv.DistanceMask5, gocv.DistanceLabelCComp)

    // 2. Find the Mode ONLY along the skeleton line
    counts := make(map[int]int)
    for y := 0; y < skeleton.Rows(); y++ {
        for x := 0; x < skeleton.Cols(); x++ {
            // ONLY sample if this pixel is part of the skeleton
            if skeleton.GetUCharAt(y, x) > 0 {
                d := int(math.Round(float64(distMap.GetFloatAt(y, x))))
                if d > 0 {
                    counts[d]++
                }
            }
        }
    }

    modeWidth := 0
    maxCount := 0
    for width, count := range counts {
        if count > maxCount {
            maxCount = count
            modeWidth = width
        }
    }

    // DEBUG: This should now show a much higher number (half the track width)
    fmt.Println("Corrected mode width (radius): ", modeWidth)

    // 3. Draw using the corrected radius
    restored := gocv.NewMatWithSize(thickTrack.Rows(), thickTrack.Cols(), gocv.MatTypeCV8UC1)
    white := color.RGBA{255, 255, 255, 0}

    for y := 0; y < skeleton.Rows(); y++ {
        for x := 0; x < skeleton.Cols(); x++ {
            if skeleton.GetUCharAt(y, x) > 0 {
                gocv.Circle(&restored, image.Point{X: x, Y: y}, modeWidth, white, -1)
            }
        }
    }

    return restored
}
