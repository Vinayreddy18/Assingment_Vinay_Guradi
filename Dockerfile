# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

# Module files first for layer caching. (No third-party deps, but this keeps
# the cache warm if any are added later.)
COPY go.mod ./
RUN go mod download

COPY . .
# Static binary so it runs on a minimal base image.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
    -o /out/samadhan ./cmd/samadhan

# ---- runtime stage ----
FROM alpine:3.20
# Non-root user and CA certs (needed for the live Anthropic provider over TLS).
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/samadhan /app/samadhan
COPY web /app/web

ENV SAMADHAN_ADDR=:8080 \
    SAMADHAN_WEB_DIR=/app/web
EXPOSE 8080
USER app

# Runs offline by default; set ANTHROPIC_API_KEY to use the live model.
ENTRYPOINT ["/app/samadhan"]
