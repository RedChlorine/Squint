package main

import (
	"fmt"
	"net/http"
)

// Response structure to ensure we don't leak excessive data
type OCRResponse struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
}

func main() {
	// secure POST endpoint
	http.HandleFunc("/api/v2/ocr", handleOCRUpload)

	fmt.Println("[INFO] Squint OCR service starting on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil{
		fmt.Printf("[ERROR] Failed to start server: %v\n", err)
	}
}

handleOCRUpload(writer http.ResponseWriter, requester *http.Request) {
	// Method Restriciton: Only allow POST requests
	if requester.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		json.newEncoder(writer).Encode(OCRResponse{
			Success: false,
			Error:   "Method not allowed. Please use POST.",
		})
		return
	}

	// Memory Protection: Limit the size of the request body to 10MB
	if err := requester.ParseMultipartForm(10 << 20); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(OCRResponse{
			Success: false,
			Error:   "Failed to parse form data. File is larger than 10MB, or corrupted.",
		})
		return
	}

	// Extract the file from the form data
	file, header, err := requester.FormFile("image")
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(writer).Encode(OCRResponse{
			Success: false,
			Error:   "No image provided in the 'image' field.",
		})
		return
	}
	defer file.Close()

	fmt.Printf("[INFO] Received file: %s (%d bytes)\n", header.Filename, header.Size)

}
