package main

import (
	"fmt"
	"io"

	"github.com/otiai10/gosseract/v2"
)

// Config defines the configuration for the OCR engine, allowing for future extensibility.
type Config struct {
	EnablePreprocessing bool `json:"enable_preprocessing"` // Whether to apply preprocessing steps to the image
}

// TesseractEngine
type TesseractEngine struct {
	Cfg Config // config injection
}

// Process satisfies the OCREngine interface by processing the image and returning the extracted text.
func (tEngine *TesseractEngine) ProcessImage(image io.Reader) (string, error) {
	fmt.Println("[INFO] Processing image with gosseract...")

	// 1. Read the image stream into memory
	imgBytes, err := io.ReadAll(image)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Optional: If preprocessing is enabled, apply any image transformations here (e.g., grayscale, thresholding).
	if tEngine.Cfg.EnablePreprocessing {
		fmt.Println("[INFO] Preprocessing image...")
	} else {
		fmt.Println("[INFO] Skipping preprocessing as per configuration.")
	}

	// 2. Initialize the gosseract client
	client := gosseract.NewClient()
	defer client.Close() // Ensure we free up the C++ memory when done

	// Force Tesseract to treat the image as a single, uniform block of text (PSM 6).
	// This stops it from trying to read UI elements or scattered artifacts as columns.
	client.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)

	// 3. Hand the bytes to the engine
	err = client.SetImageFromBytes(imgBytes)
	if err != nil {
		return "", fmt.Errorf("gosseract failed to set image: %w", err)
	}

	// 4. Execute the OCR
	text, err := client.Text()
	if err != nil {
		return "", fmt.Errorf("gosseract failed to extract text: %w", err)
	}

	return text, nil
}
