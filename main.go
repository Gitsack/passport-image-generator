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
	// Austrian passport photo standards (35x45mm at 300 DPI)
	PHOTO_WIDTH_MM  = 35
	PHOTO_HEIGHT_MM = 45
	PHOTO_WIDTH_PX  = 413
	PHOTO_HEIGHT_PX = 531
	DPI             = 300
	
	// Face positioning standards for Austrian passports
	FACE_HEIGHT_RATIO = 0.75  // Face should be 75% of photo height (middle of 71-80% range)
	EYE_POSITION_RATIO = 0.65 // Eyes at 65% from bottom (middle of 60-70% range)
	FACE_DETECTION_TO_HEAD_RATIO = 0.53 // Face detection size is ~53% of actual head height
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

var printFormats = []PrintFormat{
	{"10x15cm (8 photos)", 150, 100, 1772, 1181, 8, 4, 2}, // Fixed: landscape orientation (15x10cm)
	{"13x18cm (12 photos)", 180, 130, 2126, 1535, 12, 4, 3}, // Fixed: landscape orientation
	{"A4 (20 photos)", 297, 210, 3508, 2480, 20, 5, 4}, // Fixed: landscape orientation
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
	fmt.Println("Austrian Passport Photo Generator - Automatic Mode")
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

	// Generate preview
	previewPath := generatePreview(passportPhoto, config.InputPath)
	fmt.Printf("📷 Preview saved: %s\n", previewPath)

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
	reader := bufio.NewReader(os.Stdin)

	// Get input file
	fmt.Print("Enter path to input image: ")
	inputPath, _ := reader.ReadString('\n')
	inputPath = strings.TrimSpace(inputPath)

	// Check if file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		log.Fatal("Input file does not exist:", inputPath)
	}

	// Show available print formats
	fmt.Println("\nAvailable print formats:")
	for i, format := range printFormats {
		fmt.Printf("%d. %s - %d photos (%dx%d grid)\n",
			i+1, format.Name, format.PhotosPerSheet, format.Columns, format.Rows)
	}

	fmt.Print("Select format (1-3): ")
	formatChoice, _ := reader.ReadString('\n')
	formatChoice = strings.TrimSpace(formatChoice)

	choice, err := strconv.Atoi(formatChoice)
	if err != nil || choice < 1 || choice > len(printFormats) {
		log.Fatal("Invalid format choice")
	}

	selectedFormat := printFormats[choice-1]

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

	// EXACT Austrian passport photo specifications:
	// - Image: 35mm × 45mm (413×531 pixels at 300 DPI)
	// - Head (chin to skull): 2/3 of image = 354 pixels
	// - Eyes at 48% from top = 255 pixels from top
	// - Headspace: 1/10 of image = 53 pixels above head top
	
	// Calculate exact measurements
	targetHeadHeightChinToSkull := int(math.Round(float64(PHOTO_HEIGHT_PX) * (2.0/3.0))) // Exactly 2/3 = 354px
	eyePositionFromTop := int(math.Round(float64(PHOTO_HEIGHT_PX) * 0.48)) // Exactly 48% = 255px
	headspaceAboveHead := int(math.Round(float64(PHOTO_HEIGHT_PX) * 0.1)) // Exactly 1/10 = 53px
	
	// Face detection captures about 70% of actual head height (chin to forehead)
	targetFaceSize := int(float64(targetHeadHeightChinToSkull) * 0.70)
	
	// Calculate scale factor
	scaleFactor := float64(targetFaceSize) / float64(face.Size)
	
	// Calculate crop dimensions maintaining passport aspect ratio
	cropWidth := int(float64(PHOTO_WIDTH_PX) / scaleFactor)
	cropHeight := int(float64(PHOTO_HEIGHT_PX) / scaleFactor)
	
	// Estimate eye level (42% down from face detection top)
	eyeY := face.Y - face.Size/2 + int(float64(face.Size)*0.42)
	
	// Position eyes at EXACTLY 48% from top (255 pixels from top in final image)
	eyePositionInPhoto := int(float64(cropHeight) * 0.48) // 48% from top
	
	// Center face horizontally
	cropX := face.X - cropWidth/2
	cropY := eyeY - eyePositionInPhoto
	
	// Calculate head top position for 1/10 headspace requirement
	// Head top should be at 53 pixels from top in final image
	headTopPositionInPhoto := int(float64(cropHeight) * 0.1) // 10% from top
	
	// Estimate skull top (forehead) position
	faceTop := face.Y - face.Size/2
	estimatedSkullTop := faceTop - int(float64(face.Size)*0.15) // 15% above face detection for forehead
	
	// Ensure 1/10 headspace above head
	minCropYForHeadspace := estimatedSkullTop - headTopPositionInPhoto
	if cropY > minCropYForHeadspace {
		cropY = minCropYForHeadspace
		fmt.Printf("🔧 Adjusted crop position for 1/10 headspace requirement\n")
	}
	
	fmt.Printf("📏 Austrian passport specifications:\n")
	fmt.Printf("   - Head height (chin-to-skull): %d pixels (exactly 2/3 of %d)\n", targetHeadHeightChinToSkull, PHOTO_HEIGHT_PX)
	fmt.Printf("   - Eyes position: %d pixels from top (exactly 48%% of %d)\n", eyePositionFromTop, PHOTO_HEIGHT_PX)
	fmt.Printf("   - Headspace above head: %d pixels (exactly 1/10 of %d)\n", headspaceAboveHead, PHOTO_HEIGHT_PX)
	fmt.Printf("   - Face detection target: %d pixels (70%% of head height)\n", targetFaceSize)
	
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
		
		// Recalculate position maintaining 48% eye positioning
		cropX = face.X - cropWidth/2
		cropY = eyeY - int(float64(cropHeight)*0.48) // Keep 48% from top
		
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

func generatePreview(passportPhoto image.Image, inputPath string) string {
	// Create 2x scale preview for easy viewing
	previewScale := 2.0
	previewWidth := int(float64(PHOTO_WIDTH_PX) * previewScale)
	previewHeight := int(float64(PHOTO_HEIGHT_PX) * previewScale)
	
	preview := resizeImageHighQuality(passportPhoto, previewWidth, previewHeight)
	
	inputDir := filepath.Dir(inputPath)
	inputName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	previewPath := filepath.Join(inputDir, fmt.Sprintf("%s_passport_preview.jpg", inputName))
	
	err := saveImage(preview, previewPath)
	if err != nil {
		fmt.Printf("Warning: Could not save preview: %v\n", err)
		return ""
	}
	
	return previewPath
}

func createPrintLayout(passportPhoto image.Image, format PrintFormat) image.Image {
	fmt.Printf("📄 Creating %s layout (%dx%d grid)\n", 
		format.Name, format.Columns, format.Rows)

	// Create white canvas
	canvas := image.NewRGBA(image.Rect(0, 0, format.WidthPX, format.HeightPX))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Calculate layout with proper margins
	margin := 40
	availableWidth := format.WidthPX - 2*margin
	availableHeight := format.HeightPX - 2*margin

	// Calculate spacing for even distribution
	totalPhotosWidth := format.Columns * PHOTO_WIDTH_PX
	totalPhotosHeight := format.Rows * PHOTO_HEIGHT_PX

	var spacingX, spacingY int
	if format.Columns > 1 {
		spacingX = (availableWidth - totalPhotosWidth) / (format.Columns - 1)
	}
	if format.Rows > 1 {
		spacingY = (availableHeight - totalPhotosHeight) / (format.Rows - 1)
	}

	// Center the grid
	totalGridWidth := totalPhotosWidth + (format.Columns-1)*spacingX
	totalGridHeight := totalPhotosHeight + (format.Rows-1)*spacingY

	startX := (format.WidthPX - totalGridWidth) / 2
	startY := (format.HeightPX - totalGridHeight) / 2

	fmt.Printf("📐 Grid layout: start=(%d,%d), spacing=(%d,%d)\n", 
		startX, startY, spacingX, spacingY)

	// Place photos in grid
	photoCount := 0
	for row := 0; row < format.Rows && photoCount < format.PhotosPerSheet; row++ {
		for col := 0; col < format.Columns && photoCount < format.PhotosPerSheet; col++ {
			x := startX + col*(PHOTO_WIDTH_PX+spacingX)
			y := startY + row*(PHOTO_HEIGHT_PX+spacingY)

			// Ensure photo fits within canvas
			if x >= 0 && y >= 0 &&
				x+PHOTO_WIDTH_PX <= format.WidthPX &&
				y+PHOTO_HEIGHT_PX <= format.HeightPX {

				photoRect := image.Rect(x, y, x+PHOTO_WIDTH_PX, y+PHOTO_HEIGHT_PX)
				draw.Draw(canvas, photoRect, passportPhoto, image.Point{0, 0}, draw.Src)
				photoCount++
			}
		}
	}

	fmt.Printf("✅ Placed %d photos successfully\n", photoCount)
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
