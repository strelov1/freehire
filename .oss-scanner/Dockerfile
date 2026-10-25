# Build image for Anthropic's OSS Scanner (github.com/anthropics/oss-scanner).
#
# The scanner builds this with the repository as the build context and audits the result OFFLINE, so
# everything the backend, its workers, the web app and their tests need is fetched here. threat_model.md,
# next to this file, says what to attack and how to bring the stack up.
#
# The integration tests (`-tags=integration`) start Postgres through testcontainers, which needs Docker and
# will not run inside the scanner. The image instead carries the same services docker-compose.yml runs —
# Postgres 18 with pgvector, Redis, Meilisearch — so the server can be started against real dependencies.
FROM golang:1.26-bookworm
ENV DEBIAN_FRONTEND=noninteractive

# Postgres 18 + pgvector come from the PGDG repository: bookworm ships neither that major nor the extension,
# and the migrations run `CREATE EXTENSION vector`. poppler-utils backs CV text extraction (pdftotext).
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gnupg lsb-release xz-utils \
 && install -d /usr/share/postgresql-common/pgdg \
 && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
 && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends postgresql-18 postgresql-18-pgvector redis-server poppler-utils \
 && rm -rf /var/lib/apt/lists/*
ENV PATH=/usr/lib/postgresql/18/bin:$PATH

# Meilisearch at the version docker-compose.yml pins, and typst at the version the Dockerfile pins (CV PDFs).
ARG MEILI_VERSION=v1.49.0
ARG TYPST_VERSION=0.15.0
RUN curl -fsSL "https://github.com/meilisearch/meilisearch/releases/download/${MEILI_VERSION}/meilisearch-linux-amd64" -o /usr/local/bin/meilisearch \
 && chmod 0755 /usr/local/bin/meilisearch \
 && curl -fsSL "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" -o /tmp/typst.tar.xz \
 && tar -xJf /tmp/typst.tar.xz -C /tmp \
 && install -m 0755 /tmp/typst-x86_64-unknown-linux-musl/typst /usr/local/bin/typst \
 && rm -rf /tmp/typst*
ENV TYPST_BIN=/usr/local/bin/typst

# Node 24 + pnpm for the SvelteKit app under web/ and the design-system package it links.
ARG NODE_VERSION=24.11.0
# A failed download still fails the build without pipefail: tar rejects the empty stream.
# hadolint ignore=DL4006
RUN curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" | tar -xJ -C /usr/local --strip-components=1 \
 && npm install -g pnpm@11.7.0

# scanner contract: the checkout lives inside the image, at /src
COPY . /src
WORKDIR /src

# Every Go binary (the server and the run-once workers) into /src/bin, and every test compiled, including the
# build-tagged ones, so their dependencies are in the module cache.
RUN go mod download \
 && go build -o /src/bin/ ./cmd/... \
 && go vet ./... \
 && go test -run '^$' -tags=integration,llmlive ./... > /dev/null

# The design system first: web/ links it and neither package manager installs a linked package's dependencies.
WORKDIR /src/design-system
RUN pnpm install --frozen-lockfile && pnpm build
WORKDIR /src/web
RUN pnpm install --frozen-lockfile && pnpm build

# Cheap tests so the log shows the image works; a failure here must not abort the build.
RUN timeout 900 pnpm test > /tmp/web-test.log 2>&1 || echo "WARNING: web tests reported failures, see /tmp/web-test.log"
WORKDIR /src
RUN timeout 1800 go test ./... > /tmp/go-test.log 2>&1 || echo "WARNING: go test reported failures, see /tmp/go-test.log"

# Offline from here: the go command must not reach a proxy or download a toolchain.
ENV GOPROXY=off \
    GOTOOLCHAIN=local
