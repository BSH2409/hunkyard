# Reference Dockerfile for running hunkyard alongside an AI coding agent
# in a sandboxed container. See README.md in this directory for the full
# explanation. Build from the repository root with:
#   docker build -t hunkyard-agent -f integrations/docker/Dockerfile .
# The image installs:
#   - hunkyard (built from this checkout)
#   - claude code (npm)

FROM golang:1.26-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/hunkyard ./cmd/hunkyard

FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code
COPY --from=builder /out/hunkyard /usr/local/bin/hunkyard

WORKDIR /workspace
# Hunkyard defaults to 0.0.0.0:5000; publish with -p 127.0.0.1:5000:5000.
EXPOSE 5000
CMD ["bash"]
