# Stage 1: Build the Go application
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker's layer caching
COPY go.mod .
COPY go.sum .

# Download dependencies
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application
# CGO_ENABLED=0 disables CGO, creating a statically linked binary
# -a ensures all packages are rebuilt
# -installsuffix cgo removes the cgo suffix from the binary name
# -ldflags="-s -w" removes debugging information and symbol table, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o /app/server .

# Stage 2: Create the final, minimal image
FROM alpine:latest

# Set the working directory
WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/server /app/server

# Expose the port your application listens on
EXPOSE 8080

ENV FILE_SERVER_URL=http://file_server_#:1234/api/fileserver \
    PORT=8080 \
    REDIS_URL=redis:6379

# Command to run the application
CMD ["./server"]