// Passport Photo Generator
//
// A configurable passport photo generator that supports different country standards.
//
// CONFIGURATION:
// To adapt for different countries, modify the constants in the configuration section:
// - PHOTO_WIDTH_MM, PHOTO_HEIGHT_MM: Photo dimensions
// - PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX: Pixel dimensions (recalculate using: mm * 300 / 25.4)
// - HEAD_HEIGHT_RATIO: Head size as fraction of photo height
// - EYE_POSITION_FROM_TOP_RATIO: Eye position from top
// - HEADSPACE_RATIO: Space above head
//
// Current configuration: Austrian/EU standard (35×45mm)

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
	"path/filepath"
	"strconv"
	"strings"

	pigo "github.com/esimov/pigo/core"
	"github.com/rwcarlsen/goexif/exif"
)

const (
	// =============================================================================
	// PASSPORT PHOTO CONFIGURATION - Modify these for different countries
	// =============================================================================
	
	// Photo dimensions (default: Austrian/EU standard 35×45mm)
	// Common alternatives:
	// - US: 51×51mm (2×2 inches)
	// - UK: 45×35mm (landscape orientation)
	// - Canada: 50×70mm
	// - India: 35×45mm
	PHOTO_WIDTH_MM  = 35   // Photo width in millimeters
	PHOTO_HEIGHT_MM = 45   // Photo height in millimeters
	
	// Print quality (300 DPI is standard for professional printing)
	DPI = 300
	
	// Pixel dimensions (calculated from mm and DPI: mm * 300 / 25.4)
	// For 35×45mm at 300 DPI: 413×531 pixels
	// To change: recalculate using: new_mm * 300 / 25.4
	PHOTO_WIDTH_PX  = 413  // 35mm * 300 DPI / 25.4 = 413px
	PHOTO_HEIGHT_PX = 531  // 45mm * 300 DPI / 25.4 = 531px
	
	// =============================================================================
	// FACE POSITIONING CONFIGURATION
	// =============================================================================
	
	// Head size as fraction of photo height (default: 3/4 for Austrian standard)
	// Common alternatives:
	// - US: 50-69% (0.5 to 0.69)
	// - UK: 70-80% (0.7 to 0.8)
	// - Canada: 31-36mm for 50×70mm photo (≈ 0.5)
	HEAD_HEIGHT_RATIO = 0.75  // Head height (chin to skull) as fraction of photo height
	
	// Eye position from top as fraction of photo height (default: 48% for Austrian)
	// This determines where the eyes should be positioned vertically
	EYE_POSITION_FROM_TOP_RATIO = 0.48  // Eyes at 48% from top of photo
	
	// Headspace above head as fraction of photo height (default: 10% for Austrian)
	HEADSPACE_RATIO = 0.1  // Space above head as fraction of photo height
	
	// Face detection calibration (how much of actual head the face detection captures)
	// This is used to scale the detected face to match the required head size
	FACE_DETECTION_TO_HEAD_RATIO = 0.70  // Face detection captures ~70% of head height
	
	// Eye level within detected face (where eyes are relative to face detection box)
	EYE_LEVEL_IN_FACE_RATIO = 0.42  // Eyes at 42% down from top of face detection
	
	// Forehead estimation (how much above face detection is the skull top)
	FOREHEAD_EXTENSION_RATIO = 0.15  // Skull extends 15% above face detection
	
	// =============================================================================
	// LAYOUT CONFIGURATION
	// =============================================================================
	
	// Minimum spacing between photos in millimeters
	MIN_SPACING_MM = 2.0  // Minimum space between photos for cutting
)

type PrintFormat struct {
	Name           string
	WidthMM        int
	HeightMM       int
	WidthPX        int
	HeightPX       int
	PhotosPerSheet int
	Columns        int
	Rows           int
}

// calculateOptimalLayout calculates the optimal grid layout for 35x45mm passport photos
// It considers both orientations of the paper and chooses the one that fits more photos
func calculateOptimalLayout(widthMM, heightMM int) (cols, rows, totalPhotos int, finalWidthMM, finalHeightMM int) {
	// Try both orientations and pick the one that fits more photos
	
	// Option 1: Original orientation
	cols1, rows1, total1 := calculateLayoutForOrientation(widthMM, heightMM)
	
	// Option 2: Rotated orientation (swap width and height)
	cols2, rows2, total2 := calculateLayoutForOrientation(heightMM, widthMM)
	
	// Choose the orientation that fits more photos
	if total1 >= total2 {
		return cols1, rows1, total1, widthMM, heightMM
	} else {
		return cols2, rows2, total2, heightMM, widthMM
	}
}

// calculateLayoutForOrientation calculates layout for a specific paper orientation
// Maximizes photo count by calculating optimal spacing
func calculateLayoutForOrientation(widthMM, heightMM int) (cols, rows, totalPhotos int) {
	// Convert mm to pixels at 300 DPI
	widthPX := int(math.Round(float64(widthMM) * 300.0 / 25.4))
	heightPX := int(math.Round(float64(heightMM) * 300.0 / 25.4))
	
	// Use configurable minimum spacing
	minSpacingPX := int(math.Round(MIN_SPACING_MM * float64(DPI) / 25.4))
	minMarginPX := minSpacingPX
	
	// Calculate maximum photos that can fit with minimum spacing
	// Formula: (paperSize - 2*margin) >= cols*photoSize + (cols-1)*spacing
	// Rearranged: cols <= (paperSize - 2*margin + spacing) / (photoSize + spacing)
	
	maxCols := (widthPX - 2*minMarginPX + minSpacingPX) / (PHOTO_WIDTH_PX + minSpacingPX)
	maxRows := (heightPX - 2*minMarginPX + minSpacingPX) / (PHOTO_HEIGHT_PX + minSpacingPX)
	
	cols = maxCols
	rows = maxRows
	totalPhotos = cols * rows
	
	// Ensure at least 1 photo can fit
	if cols < 1 || rows < 1 {
		cols, rows, totalPhotos = 1, 1, 1
	}
	
	return cols, rows, totalPhotos
}

// createDynamicPrintFormat creates a PrintFormat with optimal layout calculation
func createDynamicPrintFormat(name string, widthMM, heightMM int) PrintFormat {
	cols, rows, totalPhotos, finalWidthMM, finalHeightMM := calculateOptimalLayout(widthMM, heightMM)
	
	// Convert final dimensions to pixels
	finalWidthPX := int(math.Round(float64(finalWidthMM) * 300.0 / 25.4))
	finalHeightPX := int(math.Round(float64(finalHeightMM) * 300.0 / 25.4))
	
	// Add orientation info to name if paper was rotated
	orientationInfo := ""
	if finalWidthMM != widthMM || finalHeightMM != heightMM {
		orientationInfo = fmt.Sprintf(" [rotated to %dx%dcm]", finalWidthMM/10, finalHeightMM/10)
	}
	
	return PrintFormat{
		Name:           fmt.Sprintf("%s%s (%d photos)", name, orientationInfo, totalPhotos),
		WidthMM:        finalWidthMM,
		HeightMM:       finalHeightMM,
		WidthPX:        finalWidthPX,
		HeightPX:       finalHeightPX,
		PhotosPerSheet: totalPhotos,
		Columns:        cols,
		Rows:           rows,
	}
}

// getPredefinedFormats returns the standard print formats with dynamic calculation
func getPredefinedFormats() []PrintFormat {
	return []PrintFormat{
		createDynamicPrintFormat("10x15cm", 150, 100), // Landscape: 15x10cm
		createDynamicPrintFormat("13x18cm", 180, 130), // Landscape: 18x13cm
	}
}

type Config struct {
	InputPath   string
	OutputPath  string
	PrintFormat PrintFormat
}

type FaceDetection struct {
	X, Y, Size int
	Score      float32
}

func main() {
	fmt.Printf("Passport Photo Generator - %dx%dmm Standard\n", PHOTO_WIDTH_MM, PHOTO_HEIGHT_MM)
	fmt.Println("================================================")

	config := getConfig()

	// Load and process the image
	img, err := loadImage(config.InputPath)
	if err != nil {
		log.Fatal("Error loading image:", err)
	}

	// Auto-correct orientation from EXIF
	img = correctOrientation(img, config.InputPath)

	// Create passport photo with automatic face detection and alignment
	passportPhoto, err := createPassportPhoto(img)
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
	fmt.Printf("📐 Format: %s (%d photos in %dx%d grid)\n",
		config.PrintFormat.Name, config.PrintFormat.PhotosPerSheet,
		config.PrintFormat.Columns, config.PrintFormat.Rows)
	fmt.Println("🖨️  Ready to print!")
}

func getConfig() Config {
	var inputPath string
	var selectedFormat PrintFormat
	reader := bufio.NewReader(os.Stdin)
	
	// Check for command line argument first
	if len(os.Args) > 1 {
		inputPath = os.Args[1]
		
		// If format is provided as second argument, use it; otherwise use default 10x15cm
		if len(os.Args) > 2 {
			formatArg := os.Args[2]
			predefinedFormats := getPredefinedFormats()
			
			switch formatArg {
			case "10x15", "1":
				selectedFormat = predefinedFormats[0]
			case "13x18", "2":
				selectedFormat = predefinedFormats[1]
			default:
				fmt.Printf("Invalid format '%s'. Using default 10x15cm format.\n", formatArg)
				selectedFormat = predefinedFormats[0]
			}
		} else {
			// Default to 10x15cm format for command line usage
			predefinedFormats := getPredefinedFormats()
			selectedFormat = predefinedFormats[0]
			fmt.Printf("Using default format: %s\n", selectedFormat.Name)
		}
	} else {
		// Interactive mode
		fmt.Print("Enter path to input image: ")
		input, _ := reader.ReadString('\n')
		inputPath = strings.TrimSpace(input)
		
		// Get predefined formats with dynamic calculation
		predefinedFormats := getPredefinedFormats()

		// Show available print formats
		fmt.Println("\nAvailable print formats:")
		for i, format := range predefinedFormats {
			fmt.Printf("%d. %s - %d photos (%dx%d grid)\n",
				i+1, format.Name, format.PhotosPerSheet, format.Columns, format.Rows)
		}
		fmt.Printf("%d. Custom size (WxH cm)\n", len(predefinedFormats)+1)

		fmt.Printf("Select format (1-%d): ", len(predefinedFormats)+1)
		formatChoice, _ := reader.ReadString('\n')
		formatChoice = strings.TrimSpace(formatChoice)

		choice, err := strconv.Atoi(formatChoice)
		if err != nil || choice < 1 || choice > len(predefinedFormats)+1 {
			log.Fatal("Invalid format choice")
		}

		if choice <= len(predefinedFormats) {
			// Predefined format selected
			selectedFormat = predefinedFormats[choice-1]
		} else {
			// Custom format selected
			fmt.Print("Enter width in cm: ")
			widthStr, _ := reader.ReadString('\n')
			widthStr = strings.TrimSpace(widthStr)
			
			fmt.Print("Enter height in cm: ")
			heightStr, _ := reader.ReadString('\n')
			heightStr = strings.TrimSpace(heightStr)
			
			widthCM, err1 := strconv.Atoi(widthStr)
			heightCM, err2 := strconv.Atoi(heightStr)
			
			if err1 != nil || err2 != nil || widthCM <= 0 || heightCM <= 0 {
				log.Fatal("Invalid dimensions. Please enter positive integers for width and height in cm.")
			}
			
			// Convert cm to mm for internal calculation
			widthMM := widthCM * 10
			heightMM := heightCM * 10
			
			selectedFormat = createDynamicPrintFormat(fmt.Sprintf("%dx%dcm", widthCM, heightCM), widthMM, heightMM)
			
			fmt.Printf("📐 Custom format: %s\n", selectedFormat.Name)
		}
	}

	// Check if file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		log.Fatal("Input file does not exist:", inputPath)
	}

	// Generate output filename
	inputDir := filepath.Dir(inputPath)
	inputName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(inputDir, fmt.Sprintf("%s_passport_photos_%s.jpg",
		inputName, strings.ReplaceAll(selectedFormat.Name, " ", "_")))

	return Config{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		PrintFormat: selectedFormat,
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

func createPassportPhoto(img image.Image) (image.Image, error) {
	fmt.Println("🔍 Detecting face...")
	
	// Try face detection first
	face, err := detectFace(img)
	if err != nil {
		fmt.Println("⚠️  Face detection failed, using smart center crop")
		return createPassportPhotoFallback(img), nil
	}

	fmt.Printf("✅ Face detected at (%d,%d) with size %d\n", face.X, face.Y, face.Size)
	
	// Create passport photo with proper Austrian alignment
	result := alignFaceForPassport(img, face)
	
	fmt.Println("✅ Face aligned according to Austrian passport standards")
	return result, nil
}

func detectFace(img image.Image) (*FaceDetection, error) {
	// Check if cascade file exists
	cascadePath := "facefinder"
	if _, err := os.Stat(cascadePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("face detection model not found - please download with: curl -L https://github.com/esimov/pigo/raw/master/cascade/facefinder -o facefinder")
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

	// Resize image for face detection if too large
	var resizedImg image.Image
	var scaleFactor float64 = 1.0
	maxDimension := 1200

	if origWidth > maxDimension || origHeight > maxDimension {
		if origWidth > origHeight {
			scaleFactor = float64(maxDimension) / float64(origWidth)
		} else {
			scaleFactor = float64(maxDimension) / float64(origHeight)
		}
		
		newWidth := int(float64(origWidth) * scaleFactor)
		newHeight := int(float64(origHeight) * scaleFactor)
		resizedImg = resizeImageHighQuality(img, newWidth, newHeight)
	} else {
		resizedImg = img
	}

	// Convert to grayscale for face detection
	gray := imageToGrayscale(resizedImg)
	grayBounds := gray.Bounds()
	width := grayBounds.Dx()
	height := grayBounds.Dy()

	// Convert to Pigo format
	pixels := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			grayColor := gray.GrayAt(x, y)
			pixels[y*width+x] = grayColor.Y
		}
	}

	// Face detection parameters
	minSize := 40
	maxSize := int(math.Min(float64(width), float64(height)) * 0.8)

	cParams := pigo.CascadeParams{
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

	faces := classifier.RunCascade(cParams, 0.0)
	faces = classifier.ClusterDetections(faces, 0.2)

	if len(faces) == 0 {
		return nil, fmt.Errorf("no faces detected")
	}

	// Find the best face (largest and most confident)
	var bestFace pigo.Detection
	bestScore := float64(-1000)

	for _, face := range faces {
		score := float64(face.Scale) + float64(face.Q)*100
		if score > bestScore {
			bestScore = score
			bestFace = face
		}
	}

	// Scale coordinates back to original image size
	faceDetection := &FaceDetection{
		X:     int(float64(bestFace.Col) / scaleFactor),
		Y:     int(float64(bestFace.Row) / scaleFactor),
		Size:  int(float64(bestFace.Scale) / scaleFactor),
		Score: bestFace.Q,
	}

	return faceDetection, nil
}

func alignFaceForPassport(img image.Image, face *FaceDetection) image.Image {
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Passport photo specifications using configurable constants
	// Calculate exact measurements based on configuration
	targetHeadHeightChinToSkull := int(math.Round(float64(PHOTO_HEIGHT_PX) * HEAD_HEIGHT_RATIO))
	eyePositionFromTop := int(math.Round(float64(PHOTO_HEIGHT_PX) * EYE_POSITION_FROM_TOP_RATIO))
	headspaceAboveHead := int(math.Round(float64(PHOTO_HEIGHT_PX) * HEADSPACE_RATIO))
	
	// Face detection captures a portion of actual head height (chin to forehead)
	targetFaceSize := int(float64(targetHeadHeightChinToSkull) * FACE_DETECTION_TO_HEAD_RATIO)
	
	// Calculate scale factor
	scaleFactor := float64(targetFaceSize) / float64(face.Size)
	
	// Calculate crop dimensions maintaining passport aspect ratio
	cropWidth := int(float64(PHOTO_WIDTH_PX) / scaleFactor)
	cropHeight := int(float64(PHOTO_HEIGHT_PX) / scaleFactor)
	
	// Estimate eye level within detected face
	eyeY := face.Y - face.Size/2 + int(float64(face.Size)*EYE_LEVEL_IN_FACE_RATIO)
	
	// Position eyes at configured position from top
	eyePositionInPhoto := int(float64(cropHeight) * EYE_POSITION_FROM_TOP_RATIO)
	
	// Center face horizontally
	cropX := face.X - cropWidth/2
	cropY := eyeY - eyePositionInPhoto
	
	// Calculate head top position for headspace requirement
	headTopPositionInPhoto := int(float64(cropHeight) * HEADSPACE_RATIO)
	
	// Estimate skull top (forehead) position
	faceTop := face.Y - face.Size/2
	estimatedSkullTop := faceTop - int(float64(face.Size)*FOREHEAD_EXTENSION_RATIO)
	
	// Ensure configured headspace above head
	minCropYForHeadspace := estimatedSkullTop - headTopPositionInPhoto
	if cropY > minCropYForHeadspace {
		cropY = minCropYForHeadspace
		fmt.Printf("🔧 Adjusted crop position for headspace requirement\n")
	}
	
	fmt.Printf("📏 Passport photo specifications:\n")
	fmt.Printf("   - Photo size: %dx%dmm (%dx%d pixels at %d DPI)\n", PHOTO_WIDTH_MM, PHOTO_HEIGHT_MM, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX, DPI)
	fmt.Printf("   - Head height (chin-to-skull): %d pixels (%.1f%% of %d)\n", targetHeadHeightChinToSkull, HEAD_HEIGHT_RATIO*100, PHOTO_HEIGHT_PX)
	fmt.Printf("   - Eyes position: %d pixels from top (%.1f%% of %d)\n", eyePositionFromTop, EYE_POSITION_FROM_TOP_RATIO*100, PHOTO_HEIGHT_PX)
	fmt.Printf("   - Headspace above head: %d pixels (%.1f%% of %d)\n", headspaceAboveHead, HEADSPACE_RATIO*100, PHOTO_HEIGHT_PX)
	fmt.Printf("   - Face detection target: %d pixels (%.1f%% of head height)\n", targetFaceSize, FACE_DETECTION_TO_HEAD_RATIO*100)
	
	// Boundary adjustments
	if cropX < 0 {
		cropX = 0
	}
	if cropY < 0 {
		cropY = 0
	}
	if cropX+cropWidth > imgWidth {
		cropX = imgWidth - cropWidth
	}
	if cropY+cropHeight > imgHeight {
		cropY = imgHeight - cropHeight
	}
	
	// Handle case where crop is larger than image
	if cropWidth > imgWidth || cropHeight > imgHeight {
		// Scale down crop while maintaining aspect ratio
		scaleX := float64(imgWidth) / float64(cropWidth)
		scaleY := float64(imgHeight) / float64(cropHeight)
		scale := math.Min(scaleX, scaleY) * 0.95
		
		cropWidth = int(float64(cropWidth) * scale)
		cropHeight = int(float64(cropHeight) * scale)
		
		// Recalculate position maintaining configured eye positioning
		cropX = face.X - cropWidth/2
		cropY = eyeY - int(float64(cropHeight)*EYE_POSITION_FROM_TOP_RATIO)
		
		// Final boundary check
		if cropX < 0 { cropX = 0 }
		if cropY < 0 { cropY = 0 }
		if cropX+cropWidth > imgWidth { cropX = imgWidth - cropWidth }
		if cropY+cropHeight > imgHeight { cropY = imgHeight - cropHeight }
	}

	fmt.Printf("📐 Face alignment: crop %dx%d at (%d,%d), scale %.2f\n", 
		cropWidth, cropHeight, cropX, cropY, scaleFactor)

	// Create cropped image
	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+cropX, bounds.Min.Y+cropY,
		bounds.Min.X+cropX+cropWidth, bounds.Min.Y+cropY+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)

	// Resize to exact passport dimensions
	return resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
}

func createPassportPhotoFallback(img image.Image) image.Image {
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

	// Center horizontally, position for portrait (slightly higher)
	x := (width - cropWidth) / 2
	y := int(float64(height-cropHeight) * 0.2) // 20% from top for portrait positioning

	cropped := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	srcRect := image.Rect(bounds.Min.X+x, bounds.Min.Y+y,
		bounds.Min.X+x+cropWidth, bounds.Min.Y+y+cropHeight)
	draw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, draw.Src)

	return resizeImageHighQuality(cropped, PHOTO_WIDTH_PX, PHOTO_HEIGHT_PX)
}

func createPrintLayout(passportPhoto image.Image, format PrintFormat) image.Image {
	fmt.Printf("📄 Creating %s layout (%dx%d grid)\n",
		format.Name, format.Columns, format.Rows)

	// Create white canvas
	canvas := image.NewRGBA(image.Rect(0, 0, format.WidthPX, format.HeightPX))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Calculate optimal layout with maximum photo utilization
	// Calculate spacing to distribute remaining space evenly
	
	totalPhotosWidth := format.Columns * PHOTO_WIDTH_PX
	totalPhotosHeight := format.Rows * PHOTO_HEIGHT_PX
	
	// Calculate available space for spacing and margins
	remainingWidth := format.WidthPX - totalPhotosWidth
	remainingHeight := format.HeightPX - totalPhotosHeight
	
	// Distribute remaining space: margins + spacing between photos
	// Use configurable minimum spacing, distribute rest as margins
	minSpacingPX := int(math.Round(MIN_SPACING_MM * float64(DPI) / 25.4))
	
	var spacingX, spacingY int
	var marginX, marginY int
	
	if format.Columns > 1 {
		totalSpacingWidth := (format.Columns - 1) * minSpacingPX
		marginX = (remainingWidth - totalSpacingWidth) / 2
		spacingX = minSpacingPX
		
		// If margins would be too small, increase spacing
		if marginX < minSpacingPX {
			spacingX = remainingWidth / format.Columns
			marginX = spacingX / 2
		}
	} else {
		marginX = remainingWidth / 2
		spacingX = 0
	}
	
	if format.Rows > 1 {
		totalSpacingHeight := (format.Rows - 1) * minSpacingPX
		marginY = (remainingHeight - totalSpacingHeight) / 2
		spacingY = minSpacingPX
		
		// If margins would be too small, increase spacing
		if marginY < minSpacingPX {
			spacingY = remainingHeight / format.Rows
			marginY = spacingY / 2
		}
	} else {
		marginY = remainingHeight / 2
		spacingY = 0
	}
	
	startX := marginX
	startY := marginY
	
	spacingMM := math.Min(float64(spacingX), float64(spacingY)) * 25.4 / 300.0
	marginMM := math.Min(float64(marginX), float64(marginY)) * 25.4 / 300.0

	fmt.Printf("📐 Grid layout: start=(%d,%d), spacing=%.1fmm, margin=%.1fmm\n",
		startX, startY, spacingMM, marginMM)

	// Place photos in grid with strict no-cropping policy
	photoCount := 0
	for row := 0; row < format.Rows && photoCount < format.PhotosPerSheet; row++ {
		for col := 0; col < format.Columns && photoCount < format.PhotosPerSheet; col++ {
			x := startX + col*(PHOTO_WIDTH_PX+spacingX)
			y := startY + row*(PHOTO_HEIGHT_PX+spacingY)

			// Strict boundary check: photo must fit completely within canvas
			if x >= 0 && y >= 0 &&
				x+PHOTO_WIDTH_PX <= format.WidthPX &&
				y+PHOTO_HEIGHT_PX <= format.HeightPX {

				// Place photo (35x45mm portrait orientation)
				photoRect := image.Rect(x, y, x+PHOTO_WIDTH_PX, y+PHOTO_HEIGHT_PX)
				draw.Draw(canvas, photoRect, passportPhoto, image.Point{0, 0}, draw.Src)
				photoCount++
			} else {
				fmt.Printf("⚠️  Photo at position (%d,%d) would be cropped, skipping\n", col+1, row+1)
			}
		}
	}

	fmt.Printf("✅ Placed %d photos successfully (all in 35x45mm portrait orientation)\n", photoCount)
	return canvas
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

func saveImage(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return jpeg.Encode(file, img, &jpeg.Options{Quality: 95})
}
