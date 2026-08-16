# ── Stage 1: Build ────────────────────────────────────────────────────────────
# --platform=$BUILDPLATFORM keeps the compile native and cross-compiles instead
# of emulating the target arch, which is far faster for multi-arch builds.
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Supplied by buildx; default so a plain `docker build` still works.
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /bamboo-mcp ./cmd/server

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Optional: to trust a private/corporate CA, mount or COPY your PEM files into
# certs/ and uncomment the two lines below.
#   COPY certs/*.pem /usr/local/share/ca-certificates/
#   RUN update-ca-certificates

WORKDIR /app

# Proves to the MCP Registry that whoever publishes this server name also
# controls this image. The value MUST match "name" in server.json exactly.
LABEL io.modelcontextprotocol.server.name="io.github.hmdmph/bamboo-mcp"

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
