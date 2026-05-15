package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/otiai10/gosseract/v2"
)

type OCRRequest struct {
	Text   string `json:"text"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func ocrHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	// Extract the Telegram Image URL from the request
	imageURL := request.URL.Query().Get("image_url")
	// Validate the presence of the image URL
	if imageURL == "" {
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(OCRRequest{
			Status: "error",
			Error:  "[SQUINT]: Missing image_url parameter",
		})
		return
	}

	// Downlood the image into RAM - circumvents the need to store images on disk
	response, err := http.Get(imageURL)
	// Handle potential errors during image download
	if err != nil {
		writer.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(writer).Encode(OCRRequest{
			Status: "error",
			Error:  "[SQUINT]: Failed to download image",
		})
		return
	}
	// Ensure the response body is closed after downloading the image
	defer response.Body.Close()

	// Read the image data into memory
	imgBytes, err := io.ReadAll(response.Body)
	if err != nil {
		writer.WriteHeader(http.StatusFailedDependency)
		json.NewEncoder(writer).Encode(OCRRequest{
			Status: "error",
			Error:  "[SQUINT]: Failed to read image stream",
		})
		return
	}

	// Init Tesseract OCR engine now that we have the image data in memory
	tesseractClient := gosseract.NewClient()
	defer tesseractClient.Close() // CRITICAL: Ensure the Tesseract client is properly closed to free resources

	// Config Tesseract to read L-R, T-B text (common for most languages)
	// PSM 6(Single Block Mode) is suitable for block of text, which is common in Telegram images
	tesseractClient.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)

	// Pass RAM buffer to Tesseract for OCR processing
	tesseractClient.SetImageFromBytes(imgBytes)

	// Perform OCR and capture the extracted text
	text, err := tesseractClient.Text()
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(writer).Encode(OCRRequest{
			Status: "error",
			Error:  "[SQUINT]: OCR processing failed",
		})
		return
	}

	// Respond with the extracted text in JSON format on success
	json.NewEncoder(writer).Encode(OCRRequest{
		Text:   text,
		Status: "[SQUINT]: Success",
	})
}

func main() {
	// Set up the HTTP server and route for OCR processing
	http.HandleFunc("/api/v1/ocr", ocrHandler)

	log.Println("[SQUINT]: OCR Service is running on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("[SQUINT]: Failed to start server: %v", err)
	}
}
