# ---- build stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o agent ./cmd/agent

# ---- runtime stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server .
# The agent binary reads /proc — run it directly on the host, not in a container.
# Extract it with: docker cp <container>:/app/agent ./agent
COPY --from=builder /app/agent .
COPY config.yaml .

# /data is the SQLite volume mount point.
VOLUME ["/data"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./server", "-config", "config.yaml"]
