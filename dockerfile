# --- Stage 1: Build Environment ---
FROM golang:1.26-bookworm AS builder

# Install C++ compiler and Tesseract dev headers needed by Go (CGO)
RUN apt-get update && apt-get install -y \
    libtesseract-dev \
    tesseract-ocr \
    g++

WORKDIR /app

# Copy Go dependency manifests first to leverage caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the Go source code
COPY . .

# Build the binary with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o squint main.go

# --- Stage 2: Final Runtime ---
FROM debian:bookworm-slim

# Install ONLY the runtime engine and English language data pack
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    tesseract-ocr-eng \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Bring over the compiled binary from Stage 1
COPY --from=builder /app/squint .

EXPOSE 8080

CMD ["./squint"]