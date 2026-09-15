package main

import (
	"bytes"
	"context"
	"encoding/json"
	"expvar"
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

// OCRJob structure to encapsulate the image bytes and a channel for returning results by queue pool workers.
type OCRResult struct {
	Text  string
	Error error
}

type OCRJob struct {
	FileBytes []byte
	Result    chan OCRResult
}

// SquintAPI encapsulates our endpoints and dependencies
type SquintAPI struct {
	ocrEngine OCREngine
	jobQueue  chan OCRJob
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
		jobQueue: make(chan OCRJob, 100), // Buffered channel for job queue for 100 concurrent images
	}

	apiHandler.StartWorkerPool(5) // Start 5 concurrent workers for OCR processing

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

	// Expose expvar metrics at /debug/vars
	mux.Handle("/debug/vars", expvar.Handler())

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

// handleOCRUpload handles the /api/v2/ocr endpoint, processing uploaded images and returning OCR results.
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

	// 1. Read the valid file into memory
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(writer).Encode(OCRResponse{Success: false, Error: "Failed to read file into memory"})
		return
	}

	// 2. Create the job and its unique response channel
	resultChan := make(chan OCRResult)
	job := OCRJob{
		FileBytes: fileBytes,
		Result:    resultChan,
	}

	// 3. Push the job to the queue
	api.jobQueue <- job

	// 4. Block this specific request until a worker sends the result back
	finalResult := <-resultChan

	// 5. Check if the worker encountered an error
	if finalResult.Error != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(writer).Encode(OCRResponse{
			Success: false,
			Error:   "OCR Engine failed to process the image: " + finalResult.Error.Error(),
		})
		return
	}

	// Clean Data Return (Pretty Printed)
	writer.WriteHeader(http.StatusOK)

	response := OCRResponse{
		Success: true,
		Text:    finalResult.Text,
	}

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	encoder.Encode(response)
}

func (api *SquintAPI) StartWorkerPool(numWorkers int) {
	for i := 1; i <= numWorkers; i++ {
		go func(workerID int) {
			fmt.Printf("[INFO] OCR Worker %d ready\n", workerID)

			// Listen to the queue continuously
			for job := range api.jobQueue {
				// Convert bytes back to a reader for the engine
				reader := bytes.NewReader(job.FileBytes)
				text, err := api.ocrEngine.ProcessImage(reader)

				// Send the response back through the job's unique channel
				job.Result <- OCRResult{Text: text, Error: err}
			}
		}(i)
	}
}
