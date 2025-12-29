# Build stage
FROM golang:1.24-alpine AS builder

# Install git and ca-certificates (required for go mod download)
RUN apk --no-cache add git ca-certificates

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s' -o xui-exporter ./cmd/xui-exporter

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from build stage
COPY --from=builder /app/xui-exporter .

# Expose port
EXPOSE 9100

# Run the exporter
CMD ["./xui-exporter"]
