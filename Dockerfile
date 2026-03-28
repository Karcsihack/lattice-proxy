# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy module files first to leverage Docker layer cache.
COPY go.mod ./
# COPY go.sum ./
RUN go mod download

# Copy full source.
COPY . .

# Static binary — no libc dependency, maximum portability.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o lattice-engine .

# ── Stage 2: Minimal runtime image ────────────────────────────────────────────
# alpine keeps the image tiny (~15-20 MB) while still providing a shell for
# debugging and the ca-certificates package needed for TLS to OpenAI.
FROM alpine:latest

# TLS certificates required to reach https://api.openai.com
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Only the compiled binary lands in the final image — source code stays out.
COPY --from=builder /app/lattice-engine .

EXPOSE 8080

CMD ["./lattice-engine"]
