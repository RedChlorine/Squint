package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
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

// loadConfig reads the configuration from a JSON file and returns a Config struct.
func loadConfig() Config {
	cfg := Config{EnablePreprocessing: false}
	file, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("[WARN] config.json not found, using default configuration.")
		return cfg
	}
	if err := json.Unmarshal(file, &cfg); err != nil {
		fmt.Printf("[WARN] Failed to parse config.json, using defaults: %v\n", err)
	}
	fmt.Printf("[INFO] Loaded configuration: Pre-processing = %v\n", cfg.EnablePreprocessing)
	return cfg
}

func main() {
	// 1. Load config and inject it into the engine
	appConfig := loadConfig()

	apiHandler := &SquintAPI{
		ocrEngine: &TesseractEngine{
			Cfg: appConfig,
		},
	}

	// 2. Create a custom multiplexer for our routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/ocr", apiHandler.handleOCRUpload)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"running","service":"squint-ocr"}`))
	})

	// 3. Define the HTTP server explicitly
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// 4. Start the server in a background Goroutine
	go func() {
		fmt.Println("[INFO] Squint OCR service starting on port 8080...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[FATAL] Server failed: %v\n", err)
		}
	}()

	// 5. Set up a channel to listen for OS termination signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Block main thread until a signal is received
	<-quit
	fmt.Println("\n[INFO] Shutdown signal received. Shutting down gracefully...")

	// 6. Create a deadline context to give active requests 10 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("[FATAL] Server forced to shutdown: %v\n", err)
	}

	fmt.Println("[INFO] Server exiting properly.")
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

	// Rewind the file pointer back to the beginning
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

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	encoder.Encode(response)
}
