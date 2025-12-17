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

# Use Alpine for minimal runtime (all conversions handled by dooform-libreoffice-service)
FROM alpine:latest

# Install CA certificates for HTTPS calls to LibreOffice service
RUN apk --no-cache add ca-certificates

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server .

# Expose port 8081 to the outside world
EXPOSE 8081

# Command to run the executable
CMD ["/app/server"]
