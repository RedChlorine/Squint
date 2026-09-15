package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // Register JPEG decoder
	"image/png"    // For encoding back to PNG
	"io"

	"github.com/otiai10/gosseract/v2"
)

// Config defines the configuration for the OCR engine, allowing for future extensibility.
type Config struct {
	EnablePreprocessing bool `json:"enable_preprocessing"` // Whether to apply preprocessing steps to the image
}
type TesseractEngine struct {
	Cfg Config
}

func (tEngine *TesseractEngine) ProcessImage(imageReader io.Reader) (string, error) {
	fmt.Println("[INFO] Processing image with gosseract...")

	currentCfg := tEngine.Cfg

	imgBytes, err := io.ReadAll(imageReader)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// 1. Conditional Image Matrix Manipulation
	if currentCfg.EnablePreprocessing {
		fmt.Println("[DEBUG] Pre-processing is ENABLED. Applying binarization...")
		imgBytes, err = applyBinarization(imgBytes)
		if err != nil {
			return "", fmt.Errorf("image preprocessing failed: %w", err)
		}
	} else {
		fmt.Println("[DEBUG] Pre-processing is DISABLED. Skipping filters...")
	}

	client := gosseract.NewClient()
	defer client.Close()

	client.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)

	// 2. Hand the processed bytes to Tesseract
	err = client.SetImageFromBytes(imgBytes)
	if err != nil {
		return "", fmt.Errorf("gosseract failed to set image: %w", err)
	}

	text, err := client.Text()
	if err != nil {
		return "", fmt.Errorf("gosseract failed to extract text: %w", err)
	}

	return text, nil
}

// applyBinarization converts the image to grayscale and applies a high-contrast threshold
func applyBinarization(data []byte) ([]byte, error) {
	// Decode the raw bytes into a Go image object
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	grayImg := image.NewGray(bounds)

	// Loop through every single pixel in the image grid
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			originalPixel := img.At(x, y)

			// Convert the pixel to standard grayscale
			grayPixel := color.GrayModel.Convert(originalPixel).(color.Gray)

			// Thresholding: If darker than mid-gray (128), snap to black. Else, white.
			if grayPixel.Y < 128 {
				grayImg.SetGray(x, y, color.Gray{Y: 0}) // Pure Black
			} else {
				grayImg.SetGray(x, y, color.Gray{Y: 255}) // Pure White
			}
		}
	}

	// Encode the cleaned image back into a byte buffer using lossless PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, grayImg); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
