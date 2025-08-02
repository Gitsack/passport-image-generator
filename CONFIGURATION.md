# Passport Photo Generator Configuration Guide

This document explains how to configure the passport photo generator for different countries' passport photo requirements.

## Overview

The passport photo generator is designed to be easily configurable for different countries' passport photo standards. All configuration is done by modifying constants at the top of the `main.go` file.

## Configuration Sections

### 1. Photo Dimensions

```go
// Photo dimensions (default: Austrian/EU standard 35×45mm)
PHOTO_WIDTH_MM  = 35   // Photo width in millimeters
PHOTO_HEIGHT_MM = 45   // Photo height in millimeters

// Pixel dimensions (calculated from mm and DPI: mm * 300 / 25.4)
PHOTO_WIDTH_PX  = 413  // 35mm * 300 DPI / 25.4 = 413px
PHOTO_HEIGHT_PX = 531  // 45mm * 300 DPI / 25.4 = 531px
```

**To change photo dimensions:**
1. Update `PHOTO_WIDTH_MM` and `PHOTO_HEIGHT_MM` with your country's requirements
2. Recalculate pixel dimensions using: `new_mm * 300 / 25.4`
3. Update `PHOTO_WIDTH_PX` and `PHOTO_HEIGHT_PX` with the calculated values

### 2. Face Positioning

```go
// Head size as fraction of photo height (default: 3/4 for Austrian standard)
HEAD_HEIGHT_RATIO = 0.75  // Head height (chin to skull) as fraction of photo height

// Eye position from top as fraction of photo height (default: 48% for Austrian)
EYE_POSITION_FROM_TOP_RATIO = 0.48  // Eyes at 48% from top of photo

// Headspace above head as fraction of photo height (default: 10% for Austrian)
HEADSPACE_RATIO = 0.1  // Space above head as fraction of photo height
```

### 3. Layout Configuration

```go
// Print quality (300 DPI is standard for professional printing)
DPI = 300

// Minimum spacing between photos in millimeters
MIN_SPACING_MM = 2.0  // Minimum space between photos for cutting
```

## Country-Specific Examples

### United States (2×2 inches / 51×51mm)

```go
// Photo dimensions
PHOTO_WIDTH_MM  = 51   // 2 inches = 51mm
PHOTO_HEIGHT_MM = 51   // 2 inches = 51mm
PHOTO_WIDTH_PX  = 602  // 51mm * 300 DPI / 25.4 = 602px
PHOTO_HEIGHT_PX = 602  // 51mm * 300 DPI / 25.4 = 602px

// Face positioning (US requirements: 50-69% head height)
HEAD_HEIGHT_RATIO = 0.60  // 60% head height
EYE_POSITION_FROM_TOP_RATIO = 0.50  // Eyes at 50% from top
HEADSPACE_RATIO = 0.10  // 10% headspace
```

### United Kingdom (45×35mm landscape)

```go
// Photo dimensions (landscape orientation)
PHOTO_WIDTH_MM  = 45   // Width: 45mm
PHOTO_HEIGHT_MM = 35   // Height: 35mm
PHOTO_WIDTH_PX  = 531  // 45mm * 300 DPI / 25.4 = 531px
PHOTO_HEIGHT_PX = 413  // 35mm * 300 DPI / 25.4 = 413px

// Face positioning (UK requirements: 70-80% head height)
HEAD_HEIGHT_RATIO = 0.75  // 75% head height
EYE_POSITION_FROM_TOP_RATIO = 0.50  // Eyes at 50% from top
HEADSPACE_RATIO = 0.10  // 10% headspace
```

### Canada (50×70mm)

```go
// Photo dimensions
PHOTO_WIDTH_MM  = 50   // Width: 50mm
PHOTO_HEIGHT_MM = 70   // Height: 70mm
PHOTO_WIDTH_PX  = 591  // 50mm * 300 DPI / 25.4 = 591px
PHOTO_HEIGHT_PX = 827  // 70mm * 300 DPI / 25.4 = 827px

// Face positioning (Canada requirements: 31-36mm head height ≈ 50%)
HEAD_HEIGHT_RATIO = 0.50  // 50% head height (35mm of 70mm)
EYE_POSITION_FROM_TOP_RATIO = 0.45  // Eyes at 45% from top
HEADSPACE_RATIO = 0.10  // 10% headspace
```

### India (35×45mm - same as Austrian/EU)

```go
// Photo dimensions (same as Austrian/EU standard)
PHOTO_WIDTH_MM  = 35   // Width: 35mm
PHOTO_HEIGHT_MM = 45   // Height: 45mm
PHOTO_WIDTH_PX  = 413  // 35mm * 300 DPI / 25.4 = 413px
PHOTO_HEIGHT_PX = 531  // 45mm * 300 DPI / 25.4 = 531px

// Face positioning (Indian requirements similar to Austrian)
HEAD_HEIGHT_RATIO = 0.70  // 70% head height
EYE_POSITION_FROM_TOP_RATIO = 0.50  // Eyes at 50% from top
HEADSPACE_RATIO = 0.10  // 10% headspace
```

## Advanced Configuration

### Face Detection Calibration

These constants fine-tune how the face detection algorithm maps to actual head measurements:

```go
// Face detection calibration (how much of actual head the face detection captures)
FACE_DETECTION_TO_HEAD_RATIO = 0.70  // Face detection captures ~70% of head height

// Eye level within detected face (where eyes are relative to face detection box)
EYE_LEVEL_IN_FACE_RATIO = 0.42  // Eyes at 42% down from top of face detection

// Forehead estimation (how much above face detection is the skull top)
FOREHEAD_EXTENSION_RATIO = 0.15  // Skull extends 15% above face detection
```

**Note:** These values are calibrated for the Pigo face detection library and should generally not be changed unless you're experiencing consistent alignment issues.

## Usage

### Command Line Usage

```bash
# Use default format (10x15cm)
go run main.go photo.jpg

# Specify format
go run main.go photo.jpg 10x15    # 10x15cm format
go run main.go photo.jpg 13x18    # 13x18cm format
```

### Interactive Mode

```bash
# Run without arguments for interactive mode
go run main.go
```

## Testing Your Configuration

After modifying the configuration:

1. **Test with sample images:** Use the provided sample images to verify alignment
2. **Check measurements:** The program outputs detailed measurements during processing
3. **Verify compliance:** Compare generated photos with official requirements
4. **Test different formats:** Ensure layout calculations work with various paper sizes

## Troubleshooting

### Common Issues

1. **Photos too small/large:** Adjust `HEAD_HEIGHT_RATIO`
2. **Eyes positioned incorrectly:** Modify `EYE_POSITION_FROM_TOP_RATIO`
3. **Not enough headspace:** Increase `HEADSPACE_RATIO`
4. **Layout issues:** Check `MIN_SPACING_MM` and paper size calculations

### Validation

The program outputs detailed specifications during processing:

```
📏 Passport photo specifications:
   - Photo size: 35x45mm (413x531 pixels at 300 DPI)
   - Head height (chin-to-skull): 398 pixels (75.0% of 531)
   - Eyes position: 255 pixels from top (48.0% of 531)
   - Headspace above head: 53 pixels (10.0% of 531)
```

Use these values to verify your configuration meets the requirements.

## Contributing

When adding support for a new country:

1. Research the official passport photo requirements
2. Create a configuration example in this document
3. Test with multiple sample images
4. Document any special considerations or requirements