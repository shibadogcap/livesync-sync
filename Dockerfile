# Stage 1: Build
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -tags notray -ldflags="-s -w" -o /livesync ./cmd/livesync

# Stage 2: Runtime
FROM scratch
COPY --from=builder /livesync /livesync
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/livesync"]
CMD ["--daemon"]
