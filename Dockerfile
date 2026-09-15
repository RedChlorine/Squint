FROM golang:1.26-bookworm

# Install Tesseract and its C++ dependencies
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    libtesseract-dev \
    libleptonica-dev \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Pull the high-accuracy tessdata_best English model
RUN mkdir -p /usr/share/tessdata_best && \
    wget -q https://raw.githubusercontent.com/tesseract-ocr/tessdata_best/main/eng.traineddata -O /usr/share/tessdata_best/eng.traineddata

ENV TESSDATA_PREFIX=/usr/share/tessdata_best/

WORKDIR /app

# Copy module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Compile the Go binary (CGO is enabled by default in this base image)
RUN go build -o squint-service .

# Expose the API port
EXPOSE 8080

# Start the server
CMD ["./squint-service"]