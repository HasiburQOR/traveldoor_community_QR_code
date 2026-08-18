# Build a single static binary; templates, static assets and migrations are
# embedded, so the runtime image needs nothing but the binary.
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/app /app/app
RUN mkdir -p /app/data /app/uploads && chown -R app:app /app

USER app
EXPOSE 8080

ENV ADDR=:8080 \
    DATABASE_PATH=/app/data/app.db \
    UPLOAD_DIR=/app/uploads

VOLUME ["/app/data", "/app/uploads"]

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/app"]
