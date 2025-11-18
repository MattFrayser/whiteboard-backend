FROM golang:1.24-alpine AS builder
 
WORKDIR /build
 
# Install build dependencies
RUN apk add --no-cache git ca-certificates
 
# Download dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify
 
COPY . .
 
# Go binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /whiteboard-server \
    main.go
 
# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
 
WORKDIR /app
 
# Copy CA certificates for HTTPS clients
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
 
# Copy binary
COPY --from=builder /whiteboard-server /app/server
 
# non-root user 
USER nonroot:nonroot
 
EXPOSE 8080
 
ENTRYPOINT ["/app/server"]
