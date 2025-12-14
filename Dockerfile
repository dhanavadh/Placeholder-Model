# Use the official Golang image to create a build artifact.
# This is known as a "multi-stage" build.
FROM golang:1.24-alpine as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
# -o /app/server: specifies the output file name
# ./cmd/server: specifies the main package to build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/server ./cmd/server

# ---

# Use Debian for LibreOffice support (PDF conversion)
FROM debian:bookworm-slim

# Install LibreOffice Writer and Thai fonts for PDF conversion
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice-writer \
    fonts-thai-tlwg \
    fonts-noto-cjk \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /var/cache/apt/*

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server .

# Enable LibreOffice for PDF conversion
ENV LIBREOFFICE_ENABLED=true

# Expose port 8081 to the outside world
EXPOSE 8081

# Command to run the executable
CMD ["/app/server"]
