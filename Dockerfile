# Multi-stage build for production
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./cmd/server

# Runner stage
FROM alpine:latest

WORKDIR /app

# Install CA certificates for outbound TLS calls (Cloudflare R2/S3, Google Play, Postgres TLS)
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/server /app/server
COPY --from=builder /app/migrations /app/migrations

RUN chmod +x /app/server

EXPOSE 8080

CMD ["/app/server"]
