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

# --- typst stage: fetch the pinned, statically-linked typst binary used to render CV
# PDFs (internal/cv). The musl build is fully static, so it runs on distroless/static;
# Libertinus Serif is embedded in the binary, so no fonts are bundled separately. Pinned
# to match the version verified against the ATS extraction test (local == prod output). ---
FROM alpine:3.20 AS typst
ARG TYPST_VERSION=0.15.0
RUN apk add --no-cache curl xz \
 && curl -fsSL "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" -o /tmp/typst.tar.xz \
 && tar -xJf /tmp/typst.tar.xz -C /tmp \
 && install -m 0755 /tmp/typst-x86_64-unknown-linux-musl/typst /usr/local/bin/typst \
 && /usr/local/bin/typst --version

# --- runtime stage ---
# debian-slim (not distroless/static) because résumé text extraction shells out to
# poppler's `pdftotext`, which has no static build to bundle the way typst does. The Go
# binaries stay CGO_ENABLED=0 static, so they run here unchanged. ca-certificates backs
# outbound TLS (LLM/S3/Meili/OAuth); a non-root user (uid 65532, matching the previous
# distroless nonroot) keeps volume ownership stable.
FROM debian:stable-slim
WORKDIR /app
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --system --gid 65532 nonroot \
 && useradd --system --uid 65532 --gid nonroot --home-dir /app nonroot \
 && pdftotext -v
COPY --from=build /out/hire /out/ingest /out/enrich /out/reindex /out/tg-ingest /out/tg-extract /out/reslug /out/backfill-derive /out/liveness /out/notify /out/import-collections /out/recount-companies /out/backfill-company-info /app/
# CV PDF rendering: the typst binary + the env that points the server at it. Absent this
# the CV builder still works and the PDF endpoint returns 501 (config resolves via LookPath).
COPY --from=typst /usr/local/bin/typst /app/typst
ENV TYPST_BIN=/app/typst
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/hire"]
