FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/static ./static
COPY --from=builder /app/data ./data

ENV PORT=8080
ENV LOG_LEVEL=INFO
ENV DATA_DIR=/app/data
ENV DOWNLOAD_DIR=/app/downloads

EXPOSE 8080

VOLUME ["/app/data", "/app/downloads"]

CMD ["./server"]
