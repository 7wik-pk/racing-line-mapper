package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"regexp"
	"strconv"

	"racing-line-mapper/internal/common"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

func main() {
	fmt.Println("running debug preproc script...")

	inputDir := "./input_track_maps/"
	files, err := filepath.Glob(inputDir + "*.jpg")
	if err != nil {
		fmt.Printf("Error reading input directory: %v\n", err)
		return
	}

	for _, input_path := range files {
		inputFilename := filepath.Base(input_path)

		// 1. Detect target width from filename (e.g., _10m.jpg)
		re := regexp.MustCompile(`_([0-9.]+)[mM]\.`)
		matches := re.FindStringSubmatch(inputFilename)
		if len(matches) <= 1 {
			continue // Skip files without width pattern
		}

		fmt.Printf("\n--- Processing %s ---\n", inputFilename)
		targetMeters, _ := strconv.ParseFloat(matches[1], 64)
		targetPixels := targetMeters * common.PixelsPerMeter

		// 2. Load Image
		img := gocv.IMRead(input_path, gocv.IMReadColor)
		if img.Empty() {
			fmt.Printf("Error reading %s\n", input_path)
			continue
		}

		// 3. Detect Green Dots in Original Input (Starting Locations)
		greenMask := gocv.NewMat()
		lowerGreen := gocv.NewScalar(0, 200, 0, 0)
		upperGreen := gocv.NewScalar(100, 255, 100, 0)
		gocv.InRangeWithScalar(img, lowerGreen, upperGreen, &greenMask)

		yellowMask := gocv.NewMat()
		// Yellow in BGR is (0, 255, 255). Range: B:0-100, G:200-255, R:200-255
		lowerYellow := gocv.NewScalar(0, 200, 200, 0)
		upperYellow := gocv.NewScalar(100, 255, 255, 0)
		gocv.InRangeWithScalar(img, lowerYellow, upperYellow, &yellowMask)

		// 4. Preprocessing (Inversion and Grayscale)
		gray := gocv.NewMat()
		gocv.CvtColor(img, &gray, gocv.ColorBGRToGray)
		gocv.BitwiseNot(gray, &gray)

		// Padding
		top, bottom, left, right := 64, 64, 64, 64
		black := color.RGBA{0, 0, 0, 0}
		gocv.CopyMakeBorder(gray, &gray, top, bottom, left, right, gocv.BorderConstant, black)

		paddedGreen := gocv.NewMat()
		gocv.CopyMakeBorder(greenMask, &paddedGreen, top, bottom, left, right, gocv.BorderConstant, color.RGBA{0, 0, 0, 0})

		paddedYellow := gocv.NewMat()
		gocv.CopyMakeBorder(yellowMask, &paddedYellow, top, bottom, left, right, gocv.BorderConstant, color.RGBA{0, 0, 0, 0})

		// 5. Dynamic Kernel Detection
		thresh := Threshold(gray, 150, 255)

		// Force Green and Yellow markers to be part of the track
		// This prevents holes if the markers are darker than the threshold due to color conversion
		gocv.BitwiseOr(thresh, paddedGreen, &thresh)
		gocv.BitwiseOr(thresh, paddedYellow, &thresh)

		// Use a light-touch opening just to get a reliable width reading without dissolving the track
		probe := Open(thresh, 3, 1)
		probeThin := ThinTrack(probe)
		probeThin = CloseGapsByEndpoints(probeThin)

		inputRadius := GetModeWidth(probe, probeThin)
		inputWidth := float64(inputRadius * 2)

		// Dynamic Kernel Calculation:
		// Based on manual testing: Monza (12px width) -> 13 kernel, Spa (6px width) -> 6 kernel.
		// Formula: kernelSize ≈ inputWidth * 1.1 (clamped to at least 3)
		// Floor(6 * 1.1) = 6, Floor(12 * 1.1) = 13
		kernelSize := int(math.Floor(inputWidth * 1.1))
		if kernelSize < 3 {
			kernelSize = 3
		}

		fmt.Printf("Detected input width: %.1f px. Using dynamic kernel size: %d\n", inputWidth, kernelSize)

		// Perform the real noise cleaning with the dynamic kernel
		// kernel sizes that work best for specific tracks:
		// monza - 13
		// spa - 6
		clean := Open(thresh, kernelSize, 1)

		// 6. Final Skeletonization
		thin := ThinTrack(clean)
		thin = CloseGapsByEndpoints(thin)

		// 7. Scale to Simulation Scale
		// We use the inputWidth we detected to calculate the scale factor
		scaleFactor := targetPixels / inputWidth
		fmt.Printf("Target width: %.1f px, Scaling: %.3f\n", targetPixels, scaleFactor)

		resizedThin := gocv.NewMat()
		gocv.Resize(thin, &resizedThin, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationLinear)
		gocv.Threshold(resizedThin, &resizedThin, 127, 255, gocv.ThresholdBinary)

		resizedGreen := gocv.NewMat()
		gocv.Resize(paddedGreen, &resizedGreen, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationDefault)

		resizedYellow := gocv.NewMat()
		gocv.Resize(paddedYellow, &resizedYellow, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationDefault)

		// 8. Final Reconstruction
		finalRadius := int(math.Round(targetPixels / 2.0))
		finalTrack := RestoreUniformThicknessManual(resizedThin, finalRadius)

		// Output: White track on Black background
		// Force the start/direction areas to be considered Track (White) to prevent holes
		// caused by skeletonization potentially thinning them out.
		// We use the resized masks for this.
		gocv.BitwiseOr(finalTrack, resizedGreen, &finalTrack)
		gocv.BitwiseOr(finalTrack, resizedYellow, &finalTrack)

		finalBGR := gocv.NewMat()
		gocv.CvtColor(finalTrack, &finalBGR, gocv.ColorGrayToBGR)

		// Apply Start Marker (Red)
		// We want the red dots to be on the track (White)
		redMat := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0, 0, 255, 0), finalBGR.Rows(), finalBGR.Cols(), finalBGR.Type())

		// The red dots should only appear where resizedGreen is white AND where we have track
		startMaskFinal := gocv.NewMat()
		gocv.BitwiseAnd(resizedGreen, finalTrack, &startMaskFinal)

		redMat.CopyToWithMask(&finalBGR, startMaskFinal)

		// Apply Direction Marker (Yellow)
		yellowPaint := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0, 255, 255, 0), finalBGR.Rows(), finalBGR.Cols(), finalBGR.Type())
		directionMaskFinal := gocv.NewMat()
		gocv.BitwiseAnd(resizedYellow, finalTrack, &directionMaskFinal)
		yellowPaint.CopyToWithMask(&finalBGR, directionMaskFinal)

		// 9. Save result
		outputPath := "./processed_tracks/" + inputFilename
		if ok := gocv.IMWrite(outputPath, finalBGR); !ok {
			fmt.Printf("Error writing %s\n", outputPath)
		}
		fmt.Println("Output saved to " + outputPath)

		// Cleanup
		gray.Close()
		greenMask.Close()
		yellowMask.Close()
		paddedGreen.Close()
		paddedYellow.Close()
		thresh.Close()
		probe.Close()
		probeThin.Close()
		clean.Close()
		thin.Close()
		resizedThin.Close()
		resizedGreen.Close()
		resizedYellow.Close()
		finalTrack.Close()
		finalBGR.Close()
		redMat.Close()
		startMaskFinal.Close()
		yellowPaint.Close()
		directionMaskFinal.Close()
		img.Close()
	}
}

func Threshold(img gocv.Mat, min int, max int) gocv.Mat {
	output := gocv.NewMat()
	gocv.Threshold(img, &output, float32(min), float32(max), gocv.ThresholdBinary)
	return output
}

func Open(img gocv.Mat, kernelSize int, iterations int) gocv.Mat {
	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(kernelSize, kernelSize))
	defer kernel.Close()
	output := gocv.NewMat()
	for i := 0; i < iterations; i++ {
		gocv.MorphologyEx(img, &output, gocv.MorphOpen, kernel)
		img = output
	}
	return output
}

func ThinTrack(src gocv.Mat) gocv.Mat {
	dst := gocv.NewMat()
	contrib.Thinning(src, &dst, contrib.ThinningZhangSuen)
	return dst
}

func CloseGapsByEndpoints(img gocv.Mat) gocv.Mat {
	contours := gocv.FindContours(img, gocv.RetrievalExternal, gocv.ChainApproxNone)
	if contours.Size() == 0 {
		return img.Clone()
	}
	defer contours.Close()
	type Tip struct {
		Point     image.Point
		ContourID int
	}
	var allTips []Tip
	for i := 0; i < contours.Size(); i++ {
		pts := contours.At(i).ToPoints()
		for _, p := range pts {
			if isEndpoint(img, p.X, p.Y) {
				allTips = append(allTips, Tip{p, i})
			}
		}
	}
	result := img.Clone()
	for i := 0; i < len(allTips); i++ {
		bestDist := 100.0
		bestMatchIdx := -1
		for j := 0; j < len(allTips); j++ {
			if allTips[i].ContourID == allTips[j].ContourID {
				continue
			}
			dist := math.Sqrt(math.Pow(float64(allTips[i].Point.X-allTips[j].Point.X), 2) + math.Pow(float64(allTips[i].Point.Y-allTips[j].Point.Y), 2))
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

func isEndpoint(img gocv.Mat, x, y int) bool {
	neighborCount := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			currX, currY := x+i, y+j
			if currX >= 0 && currY >= 0 && currX < img.Cols() && currY < img.Rows() {
				if img.GetUCharAt(currY, currX) > 0 {
					neighborCount++
				}
			}
		}
	}
	return neighborCount == 1
}

func GetModeWidth(thickTrack gocv.Mat, skeleton gocv.Mat) int {
	distMap := gocv.NewMat()
	defer distMap.Close()
	labels := gocv.NewMat()
	defer labels.Close()
	gocv.DistanceTransform(thickTrack, &distMap, &labels, gocv.DistL2, gocv.DistanceMask5, gocv.DistanceLabelCComp)
	counts := make(map[int]int)
	for y := 0; y < skeleton.Rows(); y++ {
		for x := 0; x < skeleton.Cols(); x++ {
			if skeleton.GetUCharAt(y, x) > 0 {
				d := int(math.Round(float64(distMap.GetFloatAt(y, x))))
				if d > 0 {
					counts[d]++
				}
			}
		}
	}
	modeWidth, maxCount := 0, 0
	for width, count := range counts {
		if count > maxCount {
			maxCount = count
			modeWidth = width
		}
	}
	return modeWidth
}

func RestoreUniformThicknessManual(skeleton gocv.Mat, radius int) gocv.Mat {
	restored := gocv.NewMatWithSize(skeleton.Rows(), skeleton.Cols(), gocv.MatTypeCV8UC1)
	for y := 0; y < skeleton.Rows(); y++ {
		for x := 0; x < skeleton.Cols(); x++ {
			if skeleton.GetUCharAt(y, x) > 0 {
				gocv.Circle(&restored, image.Point{X: x, Y: y}, radius, color.RGBA{255, 255, 255, 0}, -1)
			}
		}
	}
	return restored
}
