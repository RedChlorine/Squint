package main

import (
	"fmt"
	"io"

	"github.com/otiai10/gosseract/v2"
)

// TesseractEngine
type TesseractEngine struct {
	// TODO: Add any necessary fields for configuration, e.g., language, config options
}

// Process satisfies the OCREngine interface by processing the image and returning the extracted text.
func (tEngine *TesseractEngine) ProcessImage(image io.Reader) (string, error) {
	fmt.Println("[INFO] Processing image with gosseract...")

	// 1. Read the image stream into memory
	imgBytes, err := io.ReadAll(image)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// 2. Initialize the gosseract client
	client := gosseract.NewClient()
	defer client.Close() // Ensure we free up the C++ memory when done

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
