# Start from the official Golang base image
FROM golang:1.22.1 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -o stratum-proxy

# Start a new stage from scratch
FROM debian:bullseye-slim

# Set the Current Working Directory inside the container
WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/stratum-proxy .

# Copy the config file
COPY --from=builder /app/config.json .

# Expose port 3333 for the Stratum proxy and 2112 for Prometheus metrics
EXPOSE 3333
EXPOSE 2112

# Command to run the executable
CMD ["./stratum-proxy"]
