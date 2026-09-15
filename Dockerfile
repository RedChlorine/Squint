# Start with a solid Go compilation environment
FROM golang:1.22-bookworm

# Update the system and install Tesseract and its development libraries
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    libtesseract-dev \
    libleptonica-dev \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Create a directory for the high-accuracy training data
# and pull the tessdata_best English model directly from GitHub
RUN mkdir -p /usr/share/tessdata_best && \
    wget -q https://raw.githubusercontent.com/tesseract-ocr/tessdata_best/main/eng.traineddata -O /usr/share/tessdata_best/eng.traineddata

# Set an environment variable so Tesseract knows where to look for the good data
ENV TESSDATA_PREFIX=/usr/share/tessdata_best/

# Set up the working directory for the Go app
WORKDIR /app

# Copy the go.mod (and go.sum once we have it) to download dependencies
COPY go.mod ./
# COPY go.sum ./  <-- Uncomment this later when we add dependencies

# Copy the rest of the application source code
COPY . .

# We will add the go build command here once the main.go is written
# RUN go build -o squint-service .