# --- Stage 1: Build the Go CLI binaries ---
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache make git gcc musl-dev

WORKDIR /app

# Copy go module dependency configs
COPY go.mod go.sum ./
RUN go mod download

# Copy source trees
COPY . .

# Compile optimized static binaries
RUN go build -ldflags "-s -w" -o bin/ontology github.com/drover-org/drover-ontology/cmd/ontology && \
    go build -tags rlm -ldflags "-s -w" -o bin/rlm-ontology github.com/drover-org/drover-ontology/tools/rlm-ontology

# --- Stage 2: Distribute the compiled runtime ---
FROM alpine:3.19

RUN apk add --no-cache libc6-compat ca-certificates tzdata

WORKDIR /app

# Copy compiled binaries from builder stage
COPY --from=builder /app/bin/ontology /usr/local/bin/ontology
COPY --from=builder /app/bin/rlm-ontology /usr/local/bin/rlm-ontology
COPY --from=builder /app/references /app/references

# Expose standard visualizer port
EXPOSE 8080

# Default target directory is mapped via volume mount to /app/target
VOLUME [ "/app/target" ]

# Automatically boot the visualization server on startup targeting the mounted folder
ENTRYPOINT [ "ontology", "visualize", "--host", "0.0.0.0", "--port", "8080", "--graph", "/app/target/graph.jsonl", "--db", "/app/target/graph.db", "--schema", "/app/target/schema.yaml" ]
