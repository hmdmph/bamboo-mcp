# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bamboo-mcp cmd/server/main.go

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Optional: to trust a private/corporate CA, mount or COPY your PEM files into
# certs/ and uncomment the two lines below.
#   COPY certs/*.pem /usr/local/share/ca-certificates/
#   RUN update-ca-certificates

WORKDIR /app

COPY --from=builder /bamboo-mcp /app/bamboo-mcp

RUN addgroup -S mcp && adduser -S mcp -G mcp && \
    mkdir -p /home/mcp/.bamboo-mcp && \
    chown -R mcp:mcp /home/mcp /app

USER mcp

# SSE/HTTP is the default transport for containers
ENV MCP_TRANSPORT=sse
ENV MCP_HTTP_HOST=0.0.0.0
ENV MCP_HTTP_PORT=8080

EXPOSE 8080

# mcp-go's SSE server exposes only /sse and /message — there is no /health
# endpoint, so this checks that the server is accepting connections on the port.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD nc -z localhost 8080 || exit 1

ENTRYPOINT ["/app/bamboo-mcp"]
