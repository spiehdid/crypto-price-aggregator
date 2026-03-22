FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o cpa ./cmd/server/

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/cpa .
COPY --from=builder /build/config/config.yaml ./config/
COPY --from=builder /build/migrations/ ./migrations/

EXPOSE 8080

ENTRYPOINT ["./cpa"]
