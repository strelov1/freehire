# --- build stage ---
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache dependencies in a separate layer.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
# One image carries every binary: the HTTP server (default entrypoint) plus the
# run-once workers (ingest/enrich/reindex and the Telegram crawl/extract pair),
# which prod invokes on a schedule via `docker compose run --rm app /app/<worker>`.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/hire ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/ingest ./cmd/ingest \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/enrich ./cmd/enrich \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/reindex ./cmd/reindex \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/tg-ingest ./cmd/tg-ingest \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/tg-extract ./cmd/tg-extract \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/reslug ./cmd/reslug \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/backfill-derive ./cmd/backfill-derive \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/liveness ./cmd/liveness \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/notify ./cmd/notify \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/import-collections ./cmd/import-collections \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/recount-companies ./cmd/recount-companies \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/backfill-company-info ./cmd/backfill-company-info

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/hire /out/ingest /out/enrich /out/reindex /out/tg-ingest /out/tg-extract /out/reslug /out/backfill-derive /out/liveness /out/notify /out/import-collections /out/recount-companies /out/backfill-company-info /app/
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/hire"]
