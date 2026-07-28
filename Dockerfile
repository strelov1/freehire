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
#
# Two invocations rather than one per binary: with `-o /out/` (a directory) Go
# names each binary after its cmd dir — which is already how they're named — and
# building them together shares one dependency compile and links them in
# parallel. Cold-cache: 26s -> 17s locally, 72s -> 66s on a CI runner. Only the
# server needs an output name of its own.
#
# WORKER_PKGS is the full list by default, so an ordinary `docker build` still
# produces the complete image. The ONLY thing that overrides it is the CI page
# smoke (perf/docker-compose.smoke.yml), which boots nothing but the server and
# has no use for the workers' compile time.
ARG WORKER_PKGS="./cmd/ingest ./cmd/enrich ./cmd/reindex ./cmd/tg-ingest ./cmd/tg-extract ./cmd/backfill-derive ./cmd/liveness ./cmd/notify ./cmd/import-collections ./cmd/recount-companies ./cmd/migrate ./cmd/backfill-company-info"
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/hire ./cmd/server \
 && if [ -n "$WORKER_PKGS" ]; then \
      CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/ $WORKER_PKGS; \
    fi

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
# The whole /out, not a list of names: WORKER_PKGS decides what's in it.
COPY --from=build /out/ /app/
# The migration runner reads its *.sql files from the image (WORKDIR /app, default
# -dir migrations), so /app/migrate works the same as `go run ./cmd/migrate` on the host.
COPY --from=build /src/migrations /app/migrations
# CV PDF rendering: the typst binary + the env that points the server at it. Absent this
# the CV builder still works and the PDF endpoint returns 501 (config resolves via LookPath).
COPY --from=typst /usr/local/bin/typst /app/typst
ENV TYPST_BIN=/app/typst
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/hire"]
