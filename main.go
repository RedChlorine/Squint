package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// OCRResponse structure to ensure we don't leak excessive data
type OCRResponse struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SquintAPI encapsulates our endpoints and dependencies
type SquintAPI struct {
	ocrEngine OCREngine
}

// loadConfig reads the configuration from a JSON file and returns a Config struct. If the file is missing or malformed, it falls back to default settings.
func loadConfig() Config {
	// Set a default fallback config
	cfg := Config{EnablePreprocessing: false}

	// Attempt to read the file
	file, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("[WARN] config.json not found, using default configuration.")
		return cfg
	}

	// Parse the JSON into the struct
	if err := json.Unmarshal(file, &cfg); err != nil {
		fmt.Printf("[WARN] Failed to parse config.json, using defaults: %v\n", err)
	}

	fmt.Printf("[INFO] Loaded configuration: Pre-processing = %v\n", cfg.EnablePreprocessing)
	return cfg
}

func main() {
	// Inject the TesseractEngine into the SquintAPI
	server := &SquintAPI{
		ocrEngine: &TesseractEngine{},
	}

	// 1. Register all routes FIRST
	http.HandleFunc("/api/v2/ocr", server.handleOCRUpload)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"running","service":"squint-ocr"}`))
	})

	// 2. Start the server LAST
	fmt.Println("[INFO] Squint OCR service starting on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("[FATAL] Server failed: %v\n", err)
	}
}

func (api *SquintAPI) handleOCRUpload(writer http.ResponseWriter, requester *http.Request) {
	// Ensure the client knows we are sending JSON back
	writer.Header().Set("Content-Type", "application/json")

	// Method Restriction: Only allow POST requests
	if requester.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(writer).Encode(OCRResponse{
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

	// --- File Type Validation (The Sniffer) ---
	buff := make([]byte, 512)
	if _, err := file.Read(buff); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(writer).Encode(OCRResponse{Success: false, Error: "Failed to read file for validation"})
		return
	}

	mimeType := http.DetectContentType(buff)
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/tiff": true,
		"image/webp": true,
		"image/bmp":  true,
	}

	if !allowedTypes[mimeType] {
		fmt.Printf("[WARN] Blocked invalid file type: %s\n", mimeType)
		writer.WriteHeader(http.StatusUnsupportedMediaType)
		json.NewEncoder(writer).Encode(OCRResponse{
			Success: false,
			Error:   fmt.Sprintf("Unsupported file type: %s. Please upload PNG, JPEG, TIFF, WebP, or BMP.", mimeType),
		})
		return
	}

	// Rewind the file pointer back to the beginning so the OCR engine can read the whole image
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(writer).Encode(OCRResponse{Success: false, Error: "Failed to reset file pointer"})
		return
	}

	// --- Execute the Abstracted OCR Engine ---
	text, err := api.ocrEngine.ProcessImage(file)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(writer).Encode(OCRResponse{
			Success: false,
			Error:   "OCR Engine failed to process the image",
		})
		return
	}

	// Clean Data Return (Pretty Printed)
	writer.WriteHeader(http.StatusOK)

	response := OCRResponse{
		Success: true,
		Text:    text,
	}

	// Create an encoder and tell it to use spaces for indentation
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	encoder.Encode(response)
}
