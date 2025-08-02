package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	pigo "github.com/esimov/pigo/core"
	"github.com/rwcarlsen/goexif/exif"
)

const (
	PHOTO_WIDTH_MM  = 35
	PHOTO_HEIGHT_MM = 45
	PHOTO_WIDTH_PX  = 413
	PHOTO_HEIGHT_PX = 531
	DPI             = 300
)

type FaceAnalysis struct {
	DetectedCenter    image.Point
	AnatomicalCenter  image.Point
	EstimatedEyeLeft  image.Point
	EstimatedEyeRight image.Point
	FaceBoundingBox   image.Rectangle
	CropArea          image.Rectangle
}

type PrintFormat struct {
	Name           string
	WidthMM        int
	HeightMM       int
	WidthPX        int
	HeightPX       int
	PhotosPerSheet int
}

var dmFormats = []PrintFormat{
	{"10x15cm (Standard)", 100, 150, 1181, 1772, 12},
	{"13x18cm", 130, 180, 1535, 2126, 15},
	{"A4 (21x29.7cm)", 210, 297, 2480, 3508, 32},
	{"20x30cm", 200, 300, 2362, 3543, 48},
}

type Config struct {
	InputPath   string
	OutputPath  string
	PrintFormat PrintFormat
	AutoMode    bool
}

type FaceDetection struct {
	X, Y, Size int
	Score      float32
}

func main() {
	fmt.Println("Austrian Passport Photo Generator with AI")
	fmt.Println("========================================")

	config := getConfig()

	// Load and process the image
	img, err := loadImage(config.InputPath)
	if err != nil {
		log.Fatal("Error loading image:", err)
	}

	// Auto-correct orientation from EXIF
	img = correctOrientation(img, config.InputPath)

	// Create passport photo with automatic face detection
	passportPhoto, err := createPassportPhotoAuto(img, config.AutoMode)
	if err != nil {
		log.Fatal("Error creating passport photo:", err)
	}

	// Create print layout
	printLayout := createPrintLayout(passportPhoto, config.PrintFormat)

	// Save the result
	err = saveImage(printLayout, config.OutputPath)
	if err != nil {
		log.Fatal("Error saving image:", err)
	}

	fmt.Printf("\n✅ Success! Passport photo layout saved to: %s\n", config.OutputPath)
	fmt.Printf("📐 Format: %s (%d photos per sheet)\n", config.PrintFormat.Name, config.PrintFormat.PhotosPerSheet)
	fmt.Println("🖨️  Ready to print at DM stores!")
}

func getConfig() Config {
	reader := bufio.NewReader(os.Stdin)

	// Get input file
	fmt.Print("Enter path to input image: ")
	inputPath, _ := reader.ReadString('\n')
	inputPath = strings.TrimSpace(inputPath)

	// Check if file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		log.Fatal("Input file does not exist:", inputPath)
	}

	// Ask about auto mode
	fmt.Print("Use automatic face detection and positioning? (y/n): ")
	autoInput, _ := reader.ReadString('\n')
	autoMode := strings.ToLower(strings.TrimSpace(autoInput)) == "y"

	// Show available print formats
	fmt.Println("\nAvailable DM printing formats:")
	for i, format := range dmFormats {
		fmt.Printf("%d. %s - %d photos per sheet\n", i+1, format.Name, format.PhotosPerSheet)
	}

	fmt.Print("Select format (1-4): ")
	formatChoice, _ := reader.ReadString('\n')
	formatChoice = strings.TrimSpace(formatChoice)

	choice, err := strconv.Atoi(formatChoice)
	if err != nil || choice < 1 || choice > len(dmFormats) {
		log.Fatal("Invalid format choice")
	}

	selectedFormat := dmFormats[choice-1]

	// Generate output filename
	inputDir := filepath.Dir(inputPath)
	inputName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(inputDir, fmt.Sprintf("%s_passport_photos_%s.jpg",
		inputName, strings.ReplaceAll(selectedFormat.Name, " ", "_")))

	return Config{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		PrintFormat: selectedFormat,
		AutoMode:    autoMode,
	}
}

func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	return img, err
}

func openImage(filepath string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = exec.Command("open", filepath)
	case "linux":
		cmd = exec.Command("xdg-open", filepath)
	case "windows":
		cmd = exec.Command("start", filepath)
	default:
		fmt.Printf("Please manually open: %s\n", filepath)
		return
	}
	
	if err := cmd.Start(); err != nil {
		fmt.Printf("Could not auto-open image. Please manually open: %s\n", filepath)
	}
}

func correctOrientation(img image.Image, imagePath string) image.Image {
	file, err := os.Open(imagePath)
	if err != nil {
		return img
	}
	defer file.Close()

	exifData, err := exif.Decode(file)
	if err != nil {
		return img
	}

	orientationTag, err := exifData.Get(exif.Orientation)
	if err != nil {
		return img
	}

	orientation, err := orientationTag.Int(0)
	if err != nil {
		return img
	}

	fmt.Printf("EXIF Orientation: %d\n", orientation)

	switch orientation {
	case 3:
		return rotateImage(img, 180)
	case 6:
		return rotateImage(img, 90)
	case 8:
		return rotateImage(img, 270)
	default:
		return img
	}
}

func createPassportPhotoAuto(img image.Image, autoMode bool) (image.Image, error) {
	if autoMode {
		result := createPassportPhotoSmart(img)
		
		// Create properly scaled preview - just the single passport photo
		previewScale := 3.0 // Make it 3x larger for easy viewing
		previewWidth := int(float64(PHOTO_WIDTH_PX) * previewScale)   // 413 * 3 = 1239
		previewHeight := int(float64(PHOTO_HEIGHT_PX) * previewScale) // 531 * 3 = 1593
		
		preview := resizeImageHighQuality(result, previewWidth, previewHeight)
		
		previewPath := "passport_photo_preview.jpg"
		err := saveImage(preview, previewPath)
		if err == nil {
			fmt.Printf("\n📷 Preview saved: %s (%dx%d)\n", previewPath, previewWidth, previewHeight)
			fmt.Printf("   This shows a single passport photo at 3x scale for review\n")
		}
		
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nIs this passport photo acceptable? (y/n): ")
		response, _ := reader.ReadString('\n')
		
		if strings.ToLower(strings.TrimSpace(response)) == "n" {
			os.Remove(previewPath)
			return createPassportPhotoManualInteractive(img), nil
		}
		
		os.Remove(previewPath)
		return result, nil
	} else {
		return createPassportPhotoManualInteractive(img), nil
	}
}





func createPassportPhotoManualInteractive(img image.Image) image.Image {
	reader := bufio.NewReader(os.Stdin)
	bounds := img.Bounds()
	
	fmt.Printf("Image dimensions: %dx%d\n", bounds.Dx(), bounds.Dy())
	fmt.Print("Vertical crop position (0.0=top, 0.5=center, 1.0=bottom) [default 0.25]: ")
	
	posInput, _ := reader.ReadString('\n')
	posInput = strings.TrimSpace(posInput)
	
	verticalPos := 0.25 // Good default for passport photos
	if posInput != "" {
		if pos, err := strconv.ParseFloat(posInput, 64); err == nil && pos >= 0 && pos <= 1 {
			verticalPos = pos
		}
	}
	
	cropped := cropToPassportRatioWithPosition(img, verticalPos)
	return resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
}

func cropToPassportRatioWithPosition(img image.Image, verticalPos float64) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	targetRatio := float64(PHOTO_WIDTH_PX) / float64(PHOTO_HEIGHT_PX)
	currentRatio := float64(width) / float64(height)

	var cropWidth, cropHeight int

	if currentRatio > targetRatio {
		cropHeight = height
		cropWidth = int(float64(height) * targetRatio)
	} else {
		cropWidth = width
		cropHeight = int(float64(width) / targetRatio)
	}

	x := (width - cropWidth) / 2
	y := int(float64(height-cropHeight) * verticalPos)

	fmt.Printf("Manual crop: %dx%d at (%d,%d)\n", cropWidth, cropHeight, x, y)

	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+x, bounds.Min.Y+y, 
		bounds.Min.X+x+cropWidth, bounds.Min.Y+y+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)

	return cropped
}

func createWithFaceDetection(img image.Image) (image.Image, error) {
	// Check if cascade file exists
	cascadePath := "facefinder"
	if _, err := os.Stat(cascadePath); os.IsNotExist(err) {
		fmt.Println("Face detection model not found.")
		fmt.Println("Please download it with:")
		fmt.Println("curl -L https://github.com/esimov/pigo/raw/master/cascade/facefinder -o facefinder")
		fmt.Print("Press Enter to continue with manual mode...")
		bufio.NewReader(os.Stdin).ReadLine()
		return createPassportPhotoManual(img), nil
	}

	// Load face detection cascade
	cascadeFile, err := os.ReadFile(cascadePath)
	if err != nil {
		return nil, fmt.Errorf("error reading cascade file: %v", err)
	}

	pigoClassifier := pigo.NewPigo()
	classifier, err := pigoClassifier.Unpack(cascadeFile)
	if err != nil {
		return nil, fmt.Errorf("error unpacking cascade file: %v", err)
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()
	
	fmt.Printf("Original image: %dx%d\n", origWidth, origHeight)

	// Resize image for face detection if it's too large
	var resizedImg image.Image
	var scaleFactor float64 = 1.0
	
	maxDimension := 1500 // Maximum dimension for face detection
	
	if origWidth > maxDimension || origHeight > maxDimension {
		if origWidth > origHeight {
			scaleFactor = float64(maxDimension) / float64(origWidth)
		} else {
			scaleFactor = float64(maxDimension) / float64(origHeight)
		}
		
		newWidth := int(float64(origWidth) * scaleFactor)
		newHeight := int(float64(origHeight) * scaleFactor)
		
		fmt.Printf("Resizing for face detection: %dx%d (scale: %.3f)\n", newWidth, newHeight)
		resizedImg = resizeImageHighQuality(img, newWidth, newHeight)
	} else {
		resizedImg = img
	}

	// Convert image to grayscale
	gray := imageToGrayscale(resizedImg)
	grayBounds := gray.Bounds()
	width := grayBounds.Dx()
	height := grayBounds.Dy()

	fmt.Printf("Processing resized image: %dx%d\n", width, height)

	// Convert to Pigo format
	pixels := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			grayColor := gray.GrayAt(x, y)
			pixels[y*width+x] = grayColor.Y
		}
	}

	// Better detection parameters for high-res images
	minSize := 40  // Smaller minimum size
	maxSize := int(math.Min(float64(width), float64(height)) * 0.9) // Much larger maximum
	
	fmt.Printf("Face detection range: %d - %d pixels\n", minSize, maxSize)

	// Try multiple detection passes with different parameters
	var allFaces []pigo.Detection
	
	// Pass 1: Fine search
	cParams1 := pigo.CascadeParams{
		MinSize:     minSize,
		MaxSize:     maxSize,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   height,
			Cols:   width,
			Dim:    width,
		},
	}

	faces1 := classifier.RunCascade(cParams1, 0.0)
	faces1 = classifier.ClusterDetections(faces1, 0.2)
	allFaces = append(allFaces, faces1...)
	
	fmt.Printf("Pass 1 found %d face(s)\n", len(faces1))

	// Pass 2: Coarse search if no faces found
	if len(faces1) == 0 {
		cParams2 := pigo.CascadeParams{
			MinSize:     minSize,
			MaxSize:     maxSize,
			ShiftFactor: 0.2,  // Faster but less precise
			ScaleFactor: 1.2,
			ImageParams: pigo.ImageParams{
				Pixels: pixels,
				Rows:   height,
				Cols:   width,
				Dim:    width,
			},
		}

		faces2 := classifier.RunCascade(cParams2, -1.0) // Lower threshold
		faces2 = classifier.ClusterDetections(faces2, 0.3)
		allFaces = append(allFaces, faces2...)
		
		fmt.Printf("Pass 2 found %d additional face(s)\n", len(faces2))
	}

	fmt.Printf("Total faces found: %d\n", len(allFaces))

	if len(allFaces) == 0 {
		fmt.Println("No faces detected with automatic detection.")
		fmt.Println("This could be due to:")
		fmt.Println("- Face is at an unusual angle")
		fmt.Println("- Lighting conditions")
		fmt.Println("- Face is partially obscured")
		fmt.Println("- Image quality issues")
		fmt.Print("Switching to manual mode...")
		return createPassportPhotoManual(img), nil
	}

	// Find the best face (largest and most confident)
	var bestFace pigo.Detection
	bestScore := float64(-1000)
	
	for i, face := range allFaces {
		// Score combines size and confidence
		score := float64(face.Scale) + float64(face.Q)*100
		fmt.Printf("Face %d: x=%d, y=%d, size=%d, confidence=%.2f, score=%.2f\n", 
			i+1, face.Col, face.Row, face.Scale, face.Q, score)
		
		if score > bestScore {
			bestScore = score
			bestFace = face
		}
	}

	fmt.Printf("Using best face: x=%d, y=%d, size=%d, confidence=%.2f\n", 
		bestFace.Col, bestFace.Row, bestFace.Scale, bestFace.Q)

	// Scale face coordinates back to original image size
	faceDetection := FaceDetection{
		X:     int(float64(bestFace.Col) / scaleFactor),
		Y:     int(float64(bestFace.Row) / scaleFactor),
		Size:  int(float64(bestFace.Scale) / scaleFactor),
		Score: bestFace.Q,
	}

	fmt.Printf("Scaled to original: x=%d, y=%d, size=%d\n", 
		faceDetection.X, faceDetection.Y, faceDetection.Size)

	// NEW: Use the intelligent face analysis instead of simple cropping
	result, analysis := analyzeAndCenterFace(img, faceDetection)
	
	// Create debug image showing all the detection points
	debugImg := createDebugImage(img, analysis)
	debugPath := "face_analysis_debug.jpg"
	err = saveImage(debugImg, debugPath)
	if err == nil {
		fmt.Printf("🔍 Debug image analysis:\n")
		fmt.Printf("  🔴 Large red circle: Original face detection center\n")
		fmt.Printf("  🟢 Large green circle: Calculated anatomical center (nose/eyes)\n")
		fmt.Printf("  🔵 Blue circles: Estimated eye positions\n")
		fmt.Printf("  🟡 Thick yellow line: Facial center line (should be at nose)\n")
		fmt.Printf("  🟠 Orange line: Crop center line (where face center should be)\n")
		fmt.Printf("  🟠 Orange connecting line: Correction applied\n")
		fmt.Printf("  🔴 Red box: Face detection bounding box\n")
		fmt.Printf("  🟢 Green box: Final passport photo crop area\n")
		fmt.Printf("  Correction distance: %d pixels\n", 
			int(math.Sqrt(float64((analysis.AnatomicalCenter.X-analysis.DetectedCenter.X)*(analysis.AnatomicalCenter.X-analysis.DetectedCenter.X) + 
			(analysis.AnatomicalCenter.Y-analysis.DetectedCenter.Y)*(analysis.AnatomicalCenter.Y-analysis.DetectedCenter.Y)))))

	} else {
		fmt.Printf("Warning: Could not save debug image: %v\n", err)
	}
	
	return result, nil
}


func analyzeAndCenterFace(img image.Image, face FaceDetection) (image.Image, FaceAnalysis) {
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	fmt.Printf("Creating passport photo from %dx%d image\n", imgWidth, imgHeight)

	// PROPER passport photo sizing - face should be much smaller in the frame
	// Austrian standard: head height should be 32-36mm in a 45mm tall photo
	// This means head = ~75% of photo height, face detection size = ~50% of head
	// So face detection size should be ~37% of final photo height
	
	faceSize := face.Size
	targetPhotoHeight := int(float64(faceSize) / 0.37) // Face = 37% of photo height
	aspectRatio := float64(PHOTO_WIDTH_PX) / float64(PHOTO_HEIGHT_PX)
	targetPhotoWidth := int(float64(targetPhotoHeight) * aspectRatio)
	
	fmt.Printf("Target passport photo: %dx%d (face size: %d)\n", 
		targetPhotoWidth, targetPhotoHeight, faceSize)

	// Find anatomical center
	faceRadius := faceSize / 2
	faceBox := image.Rectangle{
		Min: image.Point{face.X - faceRadius, face.Y - faceRadius},
		Max: image.Point{face.X + faceRadius, face.Y + faceRadius},
	}
	
	// Ensure face box is within bounds
	if faceBox.Min.X < 0 { faceBox.Min.X = 0 }
	if faceBox.Min.Y < 0 { faceBox.Min.Y = 0 }
	if faceBox.Max.X > imgWidth { faceBox.Max.X = imgWidth }
	if faceBox.Max.Y > imgHeight { faceBox.Max.Y = imgHeight }

	anatomicalCenter := findAnatomicalCenter(img, faceBox)
	eyeLeft, eyeRight := estimateEyePositions(faceBox, anatomicalCenter)
	
	fmt.Printf("Face analysis: detected=(%d,%d), anatomical=(%d,%d)\n", 
		face.X, face.Y, anatomicalCenter.X, anatomicalCenter.Y)

	// Scale if needed
	cropWidth := targetPhotoWidth
	cropHeight := targetPhotoHeight
	
	if cropWidth > imgWidth || cropHeight > imgHeight {
		scale := math.Min(float64(imgWidth)/float64(cropWidth), float64(imgHeight)/float64(cropHeight)) * 0.9
		cropWidth = int(float64(cropWidth) * scale)
		cropHeight = int(float64(cropHeight) * scale)
		fmt.Printf("Scaled to fit image: %dx%d\n", cropWidth, cropHeight)
	}

	// Position crop: eyes at 57% from bottom (Austrian standard)
	eyesFromBottom := 0.57
	eyeYInCrop := int(float64(cropHeight) * eyesFromBottom)
	
	cropX := anatomicalCenter.X - cropWidth/2   // Center horizontally
	cropY := anatomicalCenter.Y - eyeYInCrop    // Position vertically

	// Boundary adjustments
	if cropX < 0 { cropX = 0 }
	if cropY < 0 { cropY = 0 }
	if cropX + cropWidth > imgWidth { cropX = imgWidth - cropWidth }
	if cropY + cropHeight > imgHeight { cropY = imgHeight - cropHeight }
	
	fmt.Printf("Final crop: %dx%d at (%d,%d)\n", cropWidth, cropHeight, cropX, cropY)

	// Create analysis for debugging
	analysis := FaceAnalysis{
		DetectedCenter:    image.Point{face.X, face.Y},
		AnatomicalCenter:  anatomicalCenter,
		EstimatedEyeLeft:  eyeLeft,
		EstimatedEyeRight: eyeRight,
		FaceBoundingBox:   faceBox,
		CropArea:          image.Rectangle{image.Point{cropX, cropY}, image.Point{cropX + cropWidth, cropY + cropHeight}},
	}

	// Crop and resize
	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+cropX, bounds.Min.Y+cropY, 
		bounds.Min.X+cropX+cropWidth, bounds.Min.Y+cropY+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)
	
	// Resize to exact passport dimensions
	final := resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
	
	return final, analysis
}

func findAnatomicalCenter(img image.Image, faceBox image.Rectangle) image.Point {
	// Convert face region to grayscale for analysis
	faceWidth := faceBox.Dx()
	faceHeight := faceBox.Dy()
	
	// Simple approach: analyze horizontal symmetry to find the center line
	centerX := faceBox.Min.X + faceWidth/2  // Start with geometric center
	
	// Analyze pixel intensities to find the most symmetric vertical line
	bestSymmetryScore := float64(-1)
	bestCenterX := centerX
	
	// Test different vertical lines near the geometric center
	searchRange := faceWidth / 8  // Search within 12.5% of face width
	for testX := centerX - searchRange; testX <= centerX + searchRange; testX++ {
		if testX < faceBox.Min.X+10 || testX > faceBox.Max.X-10 {
			continue // Stay away from edges
		}
		
		symmetryScore := calculateVerticalSymmetry(img, faceBox, testX)
		if symmetryScore > bestSymmetryScore {
			bestSymmetryScore = symmetryScore
			bestCenterX = testX
		}
	}
	
	// Estimate eye level (typically 40-45% down from top of face)
	eyeY := faceBox.Min.Y + int(float64(faceHeight)*0.42)
	
	fmt.Printf("Symmetry analysis: geometric center=%d, best symmetric line=%d (score=%.3f)\n", 
		centerX, bestCenterX, bestSymmetryScore)
	
	return image.Point{bestCenterX, eyeY}
}

func calculateVerticalSymmetry(img image.Image, faceBox image.Rectangle, centerX int) float64 {
	// Calculate how symmetric the face is around this vertical line
	if centerX <= faceBox.Min.X || centerX >= faceBox.Max.X {
		return 0
	}
	
	symmetrySum := float64(0)
	pixelCount := 0
	
	// Sample points in the eye/nose region (middle 60% of face height)
	startY := faceBox.Min.Y + int(float64(faceBox.Dy())*0.2)
	endY := faceBox.Min.Y + int(float64(faceBox.Dy())*0.8)
	
	for y := startY; y < endY; y += 2 { // Sample every 2 pixels for speed
		maxOffset := min(centerX-faceBox.Min.X, faceBox.Max.X-centerX) - 5
		
		for offset := 5; offset < maxOffset; offset += 3 {
			leftX := centerX - offset
			rightX := centerX + offset
			
			if leftX >= faceBox.Min.X && rightX < faceBox.Max.X {
				leftGray := getGrayValue(img, leftX, y)
				rightGray := getGrayValue(img, rightX, y)
				
				// Higher score for more similar intensities
				diff := abs(int(leftGray) - int(rightGray))
				symmetrySum += float64(255 - diff) // Convert difference to similarity score
				pixelCount++
			}
		}
	}
	
	if pixelCount == 0 {
		return 0
	}
	
	return symmetrySum / float64(pixelCount)
}

func getGrayValue(img image.Image, x, y int) uint8 {
	r, g, b, _ := img.At(x, y).RGBA()
	// Convert to grayscale using standard weights
	gray := (299*r + 587*g + 114*b) / 1000
	return uint8(gray >> 8)
}

func estimateEyePositions(faceBox image.Rectangle, anatomicalCenter image.Point) (image.Point, image.Point) {
	// Estimate eye positions based on typical facial proportions
	eyeY := anatomicalCenter.Y // Eyes are at the anatomical center Y
	
	// Eyes are typically about 30% of face width apart from center
	faceWidth := faceBox.Dx()
	eyeSpacing := int(float64(faceWidth) * 0.15) // 15% on each side of center
	
	eyeLeft := image.Point{anatomicalCenter.X - eyeSpacing, eyeY}
	eyeRight := image.Point{anatomicalCenter.X + eyeSpacing, eyeY}
	
	return eyeLeft, eyeRight
}

func createDebugImage(originalImg image.Image, analysis FaceAnalysis) image.Image {
	// Create a copy of the original image for debugging
	bounds := originalImg.Bounds()
	debugImg := image.NewRGBA(bounds)
	draw.Draw(debugImg, bounds, originalImg, bounds.Min, draw.Src)
	
	// Define colors
	red := color.RGBA{255, 0, 0, 255}
	green := color.RGBA{0, 255, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	yellow := color.RGBA{255, 255, 0, 255}
	orange := color.RGBA{255, 165, 0, 255}
	white := color.RGBA{255, 255, 255, 255}
	
	// Make circles much larger and more visible
	largeCircleRadius := 15
	smallCircleRadius := 10
	
	// Draw face bounding box (red rectangle) with thicker lines
	drawThickRect(debugImg, analysis.FaceBoundingBox, red, 3)
	
	// Draw crop area (green rectangle) with thick lines
	drawThickRect(debugImg, analysis.CropArea, green, 5)
	
	// Draw center line of crop area (vertical line showing where center should be)
	cropCenterX := analysis.CropArea.Min.X + analysis.CropArea.Dx()/2
	drawThickVerticalLine(debugImg, cropCenterX, analysis.CropArea.Min.Y, analysis.CropArea.Max.Y, orange, 3)
	
	// Draw detected center (large red circle)
	drawCircleWithBorder(debugImg, analysis.DetectedCenter, largeCircleRadius, red, white)
	
	// Draw anatomical center (large green circle)
	drawCircleWithBorder(debugImg, analysis.AnatomicalCenter, largeCircleRadius, green, white)
	
	// Draw estimated eyes (medium blue circles)
	drawCircleWithBorder(debugImg, analysis.EstimatedEyeLeft, smallCircleRadius, blue, white)
	drawCircleWithBorder(debugImg, analysis.EstimatedEyeRight, smallCircleRadius, blue, white)
	
	// Draw anatomical center line (thick yellow vertical line)
	drawThickVerticalLine(debugImg, analysis.AnatomicalCenter.X, 
		analysis.FaceBoundingBox.Min.Y, analysis.FaceBoundingBox.Max.Y, yellow, 5)
	
	// Draw connecting line between detected and anatomical centers
	drawLine(debugImg, analysis.DetectedCenter, analysis.AnatomicalCenter, orange, 2)
	
	return debugImg
}

// FIXED function signature and implementation
func drawThickVerticalLine(img *image.RGBA, x, y1, y2 int, c color.RGBA, thickness int) {
	for t := -thickness/2; t <= thickness/2; t++ {
		currentX := x + t
		if currentX >= 0 && currentX < img.Bounds().Dx() {
			for y := y1; y <= y2; y++ {
				if y >= 0 && y < img.Bounds().Dy() {
					img.Set(currentX, y, c)
				}
			}
		}
	}
}

func drawThickRect(img *image.RGBA, rect image.Rectangle, c color.RGBA, thickness int) {
	for t := 0; t < thickness; t++ {
		// Top and bottom lines
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if x >= 0 && x < img.Bounds().Dx() {
				// Top line
				if rect.Min.Y+t >= 0 && rect.Min.Y+t < img.Bounds().Dy() {
					img.Set(x, rect.Min.Y+t, c)
				}
				// Bottom line
				if rect.Max.Y-1-t >= 0 && rect.Max.Y-1-t < img.Bounds().Dy() {
					img.Set(x, rect.Max.Y-1-t, c)
				}
			}
		}
		// Left and right lines
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			if y >= 0 && y < img.Bounds().Dy() {
				// Left line
				if rect.Min.X+t >= 0 && rect.Min.X+t < img.Bounds().Dx() {
					img.Set(rect.Min.X+t, y, c)
				}
				// Right line
				if rect.Max.X-1-t >= 0 && rect.Max.X-1-t < img.Bounds().Dx() {
					img.Set(rect.Max.X-1-t, y, c)
				}
			}
		}
	}
}

func drawCircleWithBorder(img *image.RGBA, center image.Point, radius int, fillColor, borderColor color.RGBA) {
	// Draw filled circle
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			dist := x*x + y*y
			px := center.X + x
			py := center.Y + y
			
			if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
				if dist <= radius*radius {
					if dist <= (radius-2)*(radius-2) {
						img.Set(px, py, fillColor) // Fill
					} else {
						img.Set(px, py, borderColor) // Border
					}
				}
			}
		}
	}
}

func drawLine(img *image.RGBA, from, to image.Point, c color.RGBA, thickness int) {
	dx := abs(to.X - from.X)
	dy := abs(to.Y - from.Y)
	sx := 1
	sy := 1
	
	if from.X >= to.X {
		sx = -1
	}
	if from.Y >= to.Y {
		sy = -1
	}
	
	err := dx - dy
	x, y := from.X, from.Y
	
	for {
		// Draw thick point
		for tx := -thickness/2; tx <= thickness/2; tx++ {
			for ty := -thickness/2; ty <= thickness/2; ty++ {
				px, py := x+tx, y+ty
				if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
					img.Set(px, py, c)
				}
			}
		}
		
		if x == to.X && y == to.Y {
			break
		}
		
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x += sx
		}
		if e2 < dx {
			err += dx
			y += sy
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func createPassportPhotoSmart(img image.Image) image.Image {
	// Try face detection first
	fmt.Println("Attempting face detection...")
	if result, err := createWithFaceDetection(img); err == nil {
		return result
	}
	
	// Fallback to smart center cropping
	fmt.Println("Face detection failed, using smart center cropping...")
	return createPassportPhotoCenterWeighted(img)
}

func createPassportPhotoCenterWeighted(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	targetRatio := float64(PHOTO_WIDTH_PX) / float64(PHOTO_HEIGHT_PX)
	currentRatio := float64(width) / float64(height)

	var cropWidth, cropHeight int

	if currentRatio > targetRatio {
		cropHeight = height
		cropWidth = int(float64(height) * targetRatio)
	} else {
		cropWidth = width
		cropHeight = int(float64(width) / targetRatio)
	}

	// Center horizontally, position vertically for portrait
	x := (width - cropWidth) / 2
	y := int(float64(height-cropHeight) * 0.15) // Portrait positioning

	fmt.Printf("Smart center crop: %dx%d at (%d,%d)\n", cropWidth, cropHeight, x, y)

	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+x, bounds.Min.Y+y, 
		bounds.Min.X+x+cropWidth, bounds.Min.Y+y+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)

	return resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
}

func imageToGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, img.At(x, y))
		}
	}
	
	return gray
}

func cropAroundFaceAutomatic(img image.Image, face FaceDetection) image.Image {
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	fmt.Printf("Original image: %dx%d\n", imgWidth, imgHeight)
	fmt.Printf("Face detected: center=(%d,%d), size=%d\n", face.X, face.Y, face.Size)

	// Calculate crop dimensions
	aspectRatio := float64(PHOTO_WIDTH_PX) / float64(PHOTO_HEIGHT_PX)
	faceWidth := face.Size
	cropWidth := int(float64(faceWidth) / 0.55)
	cropHeight := int(float64(cropWidth) / aspectRatio)

	// AUTOMATIC CENTERING CORRECTION
	// Face detection often has systematic biases - let's correct them automatically
	
	detectedCenterX := face.X
	detectedCenterY := face.Y
	
	// Apply automatic corrections based on common face detection patterns:
	
	// 1. Horizontal correction: Face detection boxes often include more hair/forehead on one side
	//    Based on your measurement (42px off), typical correction is 3-7% of face size
	horizontalCorrection := int(float64(face.Size) * 0.06) // 6% of face size to the right
	
	// 2. For glasses wearers, the correction might be slightly different
	//    Glasses can shift the detected center - usually needs more rightward correction
	glassesCorrection := int(float64(face.Size) * 0.02) // Additional 2% for glasses
	totalHorizontalCorrection := horizontalCorrection + glassesCorrection
	
	// 3. Vertical correction: Estimate eye level more precisely
	//    Eyes are typically 40-45% down from the top of the face bounding box
	faceTop := detectedCenterY - face.Size/2
	estimatedEyeY := faceTop + int(float64(face.Size) * 0.42)
	
	// Apply corrections
	correctedCenterX := detectedCenterX + totalHorizontalCorrection
	correctedCenterY := estimatedEyeY
	
	fmt.Printf("Automatic corrections applied:\n")
	fmt.Printf("  Horizontal: +%d pixels (face detection bias + glasses correction)\n", totalHorizontalCorrection)
	fmt.Printf("  Vertical: eye level estimated at %d (was %d)\n", correctedCenterY, detectedCenterY)
	fmt.Printf("  Corrected center: (%d, %d)\n", correctedCenterX, correctedCenterY)
	
	// Calculate crop position for perfect centering using corrected center
	cropX := correctedCenterX - cropWidth/2
	cropY := correctedCenterY - int(float64(cropHeight)*0.65) // Eyes at 65% from bottom
	
	fmt.Printf("Initial crop calculation: %dx%d at (%d,%d)\n", cropWidth, cropHeight, cropX, cropY)
	
	// Bounds checking and adjustment
	if cropX < 0 {
		fmt.Printf("Crop extends beyond left edge, adjusting...\n")
		cropX = 0
	}
	if cropY < 0 {
		fmt.Printf("Crop extends beyond top edge, adjusting...\n")
		cropY = 0
	}
	if cropX+cropWidth > imgWidth {
		fmt.Printf("Crop extends beyond right edge, adjusting...\n")
		cropX = imgWidth - cropWidth
	}
	if cropY+cropHeight > imgHeight {
		fmt.Printf("Crop extends beyond bottom edge, adjusting...\n")
		cropY = imgHeight - cropHeight
	}

	// Handle oversized crops with maintained centering
	if cropWidth > imgWidth || cropHeight > imgHeight {
		fmt.Printf("Crop too large for image, scaling down while maintaining centering...\n")
		
		scaleX := float64(imgWidth) / float64(cropWidth)
		scaleY := float64(imgHeight) / float64(cropHeight)
		scale := math.Min(scaleX, scaleY) * 0.95
		
		// Scale dimensions
		newCropWidth := int(float64(cropWidth) * scale)
		newCropHeight := int(float64(cropHeight) * scale)
		
		fmt.Printf("Scaled crop size: %dx%d -> %dx%d\n", cropWidth, cropHeight, newCropWidth, newCropHeight)
		
		cropWidth = newCropWidth
		cropHeight = newCropHeight
		
		// Recalculate position to maintain centering
		cropX = correctedCenterX - cropWidth/2
		cropY = correctedCenterY - int(float64(cropHeight)*0.65)
		
		// Final bounds check
		if cropX < 0 { cropX = 0 }
		if cropY < 0 { cropY = 0 }
		if cropX+cropWidth > imgWidth { cropX = imgWidth - cropWidth }
		if cropY+cropHeight > imgHeight { cropY = imgHeight - cropHeight }
	}

	// Final verification and reporting
	actualFacePosX := correctedCenterX - cropX
	idealCenterX := cropWidth / 2
	centeringAccuracy := actualFacePosX - idealCenterX
	
	fmt.Printf("Final crop: %dx%d at (%d,%d)\n", cropWidth, cropHeight, cropX, cropY)
	fmt.Printf("Centering verification:\n")
	fmt.Printf("  Face center position in crop: %d pixels from left\n", actualFacePosX)
	fmt.Printf("  Ideal center position: %d pixels from left\n", idealCenterX)
	fmt.Printf("  Centering accuracy: %d pixels off (target: 0)\n", centeringAccuracy)
	
	if abs(centeringAccuracy) <= 2 {
		fmt.Printf("  ✅ Excellent centering achieved!\n")
	} else if abs(centeringAccuracy) <= 5 {
		fmt.Printf("  ✅ Good centering achieved\n")
	} else {
		fmt.Printf("  ⚠️  Centering could be improved\n")
	}

	// Create cropped image
	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+cropX, bounds.Min.Y+cropY, 
		bounds.Min.X+cropX+cropWidth, bounds.Min.Y+cropY+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)

	return resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
}

func createPassportPhotoManual(img image.Image) image.Image {
	cropped := cropToPassportRatio(img)
	return resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
}

func cropToPassportRatio(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	targetRatio := float64(PHOTO_WIDTH_PX) / float64(PHOTO_HEIGHT_PX)
	currentRatio := float64(width) / float64(height)

	var cropWidth, cropHeight int

	if currentRatio > targetRatio {
		cropHeight = height
		cropWidth = int(float64(height) * targetRatio)
	} else {
		cropWidth = width
		cropHeight = int(float64(width) / targetRatio)
	}

	x := (width - cropWidth) / 2
	y := (height - cropHeight) / 4

	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+x, bounds.Min.Y+y, 
		bounds.Min.X+x+cropWidth, bounds.Min.Y+y+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)

	return cropped
}

func rotateImage(img image.Image, degrees int) image.Image {
	bounds := img.Bounds()
	
	switch degrees {
	case 90:
		rotated := image.NewRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				rotated.Set(bounds.Dy()-(y-bounds.Min.Y)-1, x-bounds.Min.X, img.At(x, y))
			}
		}
		return rotated
	case 180:
		rotated := image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				rotated.Set(bounds.Max.X-(x-bounds.Min.X)-1, bounds.Max.Y-(y-bounds.Min.Y)-1, img.At(x, y))
			}
		}
		return rotated
	case 270:
		rotated := image.NewRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				rotated.Set(y-bounds.Min.Y, bounds.Dx()-(x-bounds.Min.X)-1, img.At(x, y))
			}
		}
		return rotated
	default:
		return img
	}
}

func resizeImageHighQuality(img image.Image, width, height int) image.Image {
	srcBounds := img.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	xRatio := float64(srcWidth) / float64(width)
	yRatio := float64(srcHeight) / float64(height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := float64(x) * xRatio
			srcY := float64(y) * yRatio
			
			x1 := int(math.Floor(srcX))
			y1 := int(math.Floor(srcY))
			x2 := int(math.Min(float64(x1+1), float64(srcWidth-1)))
			y2 := int(math.Min(float64(y1+1), float64(srcHeight-1)))
			
			c1 := img.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y1)
			c2 := img.At(srcBounds.Min.X+x2, srcBounds.Min.Y+y1)
			c3 := img.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y2)
			c4 := img.At(srcBounds.Min.X+x2, srcBounds.Min.Y+y2)
			
			r1, g1, b1, a1 := c1.RGBA()
			r2, g2, b2, a2 := c2.RGBA()
			r3, g3, b3, a3 := c3.RGBA()
			r4, g4, b4, a4 := c4.RGBA()
			
			r := (r1 + r2 + r3 + r4) / 4
			g := (g1 + g2 + g3 + g4) / 4
			b := (b1 + b2 + b3 + b4) / 4
			a := (a1 + a2 + a3 + a4) / 4
			
			dst.Set(x, y, color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)})
		}
	}

	return dst
}

func createPrintLayout(passportPhoto image.Image, format PrintFormat) image.Image {
	fmt.Printf("Creating print layout: %s (%dx%d)\n", format.Name, format.WidthPX, format.HeightPX)
	
	// Create white canvas
	canvas := image.NewRGBA(image.Rect(0, 0, format.WidthPX, format.HeightPX))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Calculate how many photos can actually fit
	margin := 30 // Margin from edges
	minSpacing := 15 // Minimum spacing between photos
	
	availableWidth := format.WidthPX - 2*margin
	availableHeight := format.HeightPX - 2*margin
	
	// Calculate maximum photos per row and column that fit without cropping
	maxPhotosPerRow := (availableWidth + minSpacing) / (PHOTO_WIDTH_PX + minSpacing)
	maxPhotosPerCol := (availableHeight + minSpacing) / (PHOTO_HEIGHT_PX + minSpacing)
	maxPhotosTotal := maxPhotosPerRow * maxPhotosPerCol
	
	// Use the smaller of requested photos or what fits
	actualPhotos := min(format.PhotosPerSheet, maxPhotosTotal)
	
	// Determine optimal grid layout
	var photosPerRow, photosPerCol int
	if actualPhotos <= 4 {
		photosPerRow = 2
		photosPerCol = 2
	} else if actualPhotos <= 6 {
		photosPerRow = 3
		photosPerCol = 2
	} else if actualPhotos <= 8 {
		photosPerRow = 4
		photosPerCol = 2
	} else if actualPhotos <= 12 {
		photosPerRow = 4
		photosPerCol = 3
	} else {
		photosPerRow = maxPhotosPerRow
		photosPerCol = (actualPhotos + photosPerRow - 1) / photosPerRow
	}
	
	// Ensure we don't exceed limits
	if photosPerRow > maxPhotosPerRow { photosPerRow = maxPhotosPerRow }
	if photosPerCol > maxPhotosPerCol { photosPerCol = maxPhotosPerCol }
	
	fmt.Printf("Grid layout: %dx%d (%d photos)\n", photosPerRow, photosPerCol, photosPerRow*photosPerCol)

	// Calculate spacing for even distribution
	totalPhotosWidth := photosPerRow * PHOTO_WIDTH_PX
	totalPhotosHeight := photosPerCol * PHOTO_HEIGHT_PX
	
	spacingX := minSpacing
	spacingY := minSpacing
	
	if photosPerRow > 1 {
		spacingX = (availableWidth - totalPhotosWidth) / (photosPerRow - 1)
	}
	if photosPerCol > 1 {
		spacingY = (availableHeight - totalPhotosHeight) / (photosPerCol - 1)
	}
	
	// Center the entire grid
	totalGridWidth := totalPhotosWidth + (photosPerRow-1)*spacingX
	totalGridHeight := totalPhotosHeight + (photosPerCol-1)*spacingY
	
	startX := (format.WidthPX - totalGridWidth) / 2
	startY := (format.HeightPX - totalGridHeight) / 2
	
	fmt.Printf("Grid positioning: start=(%d,%d), spacing=(%d,%d)\n", startX, startY, spacingX, spacingY)

	// Place photos without any cropping
	photoCount := 0
	for row := 0; row < photosPerCol && photoCount < actualPhotos; row++ {
		for col := 0; col < photosPerRow && photoCount < actualPhotos; col++ {
			x := startX + col*(PHOTO_WIDTH_PX + spacingX)
			y := startY + row*(PHOTO_HEIGHT_PX + spacingY)
			
			// Ensure photo fits completely within canvas
			if x >= 0 && y >= 0 && 
			   x + PHOTO_WIDTH_PX <= format.WidthPX && 
			   y + PHOTO_HEIGHT_PX <= format.HeightPX {
				
				photoRect := image.Rect(x, y, x+PHOTO_WIDTH_PX, y+PHOTO_HEIGHT_PX)
				draw.Draw(canvas, photoRect, passportPhoto, image.Point{0, 0}, draw.Src)
				
				fmt.Printf("Photo %d: placed at (%d,%d)\n", photoCount+1, x, y)
				photoCount++
			} else {
				fmt.Printf("Photo %d: skipped (would exceed canvas)\n", photoCount+1)
			}
		}
	}
	
	fmt.Printf("Placed %d photos successfully\n", photoCount)
	return canvas
}

// Helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}


func calculatePhotosPerRow(format PrintFormat) int {
	// Calculate based on how many photos can actually fit
	maxPhotosPerRow := (format.WidthPX - 40) / (PHOTO_WIDTH_PX + 10) // 40px total margin, 10px min spacing
	
	switch {
	case format.PhotosPerSheet <= 4:
		return min(2, maxPhotosPerRow)
	case format.PhotosPerSheet <= 9:
		return min(3, maxPhotosPerRow)
	case format.PhotosPerSheet <= 16:
		return min(4, maxPhotosPerRow)
	case format.PhotosPerSheet <= 25:
		return min(5, maxPhotosPerRow)
	default:
		return min(6, maxPhotosPerRow)
	}
}


func saveImage(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return jpeg.Encode(file, img, &jpeg.Options{Quality: 95})
}
