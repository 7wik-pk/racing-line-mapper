package main

// import (
// 	"fmt"
// 	"gocv.io/x/gocv"
// )

// func main(){
// 	// 1. Load the image
// 	img := gocv.IMRead("input.jpg", gocv.IMReadColor)
// 	if img.Empty() {
// 		fmt.Printf("Error reading image\n")
// 		return
// 	}
// 	defer img.Close()

// 	// 2. Convert to grayscale
// 	gray := gocv.NewMat()
// 	defer gray.Close()
// 	gocv.CvtColor(img, &gray, gocv.ColorBGRToGray)

// 	// 3. Apply Canny edge detection
// 	// Define thresholds (these often require tuning for different images)
// 	// Weak edges connected to strong edges are kept (hysteresis thresholding)
// 	lowThreshold := float32(50)
// 	highThreshold := float32(150)
// 	cannyOutput := gocv.NewMat()
// 	defer cannyOutput.Close()
	
// 	// The third parameter is the aperture size for the Sobel operator (e.g., 3, 5, or 7)
// 	gocv.Canny(gray, &cannyOutput, lowThreshold, highThreshold)

// 	// 4. Save the result
// 	if ok := gocv.IMWrite("output_edges.jpg", cannyOutput); !ok {
// 		fmt.Printf("Error writing image\n")
// 	}
// 	fmt.Printf("Canny edge detection applied and saved to output_edges.jpg\n")
// }