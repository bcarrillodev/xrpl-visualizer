FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o validator-service ./cmd/validator-service

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app/

# Copy binary from builder and change ownership
COPY --from=builder /app/validator-service .
RUN chown appuser:appgroup validator-service

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Run the service
CMD ["./validator-service"]
