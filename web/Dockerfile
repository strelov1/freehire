# Production image for the freehire web frontend (SvelteKit SSR, adapter-node).
#
# A single public container keeps everything same-origin for the SameSite=Lax
# auth cookie: nginx listens on :8080 and routes
#   - /api, /health  -> the Go backend (app:8080)
#   - everything else -> the Node SSR server (127.0.0.1:3000)
# The Node server fetches the Go API server-to-server via API_INTERNAL_URL.
#
# Build context is the repo root (the sibling design-system/ package is reached
# via the file:../design-system dependency):
#   docker build -f web/Dockerfile .

# --- design-system build stage ---
# Must build before web — web's file:../design-system dependency links here.
FROM node:24-alpine AS ds-build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /design-system
COPY design-system/package.json design-system/pnpm-lock.yaml design-system/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY design-system/ ./
RUN pnpm build

# --- web build stage ---
FROM node:24-alpine AS build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /src
# The file:../design-system dependency resolves to /design-system from WORKDIR /src.
COPY --from=ds-build /design-system /design-system
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# The public site URL, baked in at build time (paths.origin in vite.config.ts) — the
# SvelteKit 3 adapter no longer reads ORIGIN when the server starts. Not a secret.
ARG ORIGIN=""
ENV ORIGIN=$ORIGIN
RUN pnpm run build
# Keep only production deps for the runtime image (the build tools are no longer
# needed; adapter-node has already bundled the app).
RUN pnpm prune --prod

# --- runtime stage ---
FROM node:24-alpine
# nginx-mod-http-brotli brings ngx_brotli (filter + static). Alpine's nginx.conf
# includes /etc/nginx/modules/*.conf, so installing the package is all the wiring
# the load_module lines need. Without it, `brotli`/`brotli_static` in nginx.conf
# are unknown directives and nginx refuses to start.
# Alpine puts its module symlink beneath this otherwise root-only directory. Allow
# traversal, not writes; runtime data is kept under /tmp by nginx-main.conf.
# hadolint ignore=DL3018
RUN apk add --no-cache nginx nginx-mod-http-brotli \
    && chmod 0755 /var/lib/nginx
WORKDIR /app
COPY --from=build /src/build ./build
COPY --from=build /src/node_modules ./node_modules
COPY --from=build /src/package.json ./package.json
COPY web/nginx.conf /etc/nginx/http.d/default.conf
COPY web/nginx-main.conf /etc/nginx/nginx.conf
COPY web/docker-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Node SSR server settings. The public origin (canonical/sitemap/JSON-LD) comes from
# the ORIGIN build arg above; without one, PROTOCOL_HEADER/HOST_HEADER let the server
# derive it from nginx's forwarded headers. See web/.env.example.
ENV PORT=3000
ENV API_INTERNAL_URL=http://app:8080
ENV PROTOCOL_HEADER=x-forwarded-proto
ENV HOST_HEADER=x-forwarded-host

ENV HOME=/tmp/freehire-web/home

# Code and nginx templates stay root-owned; only runtime scratch belongs to Node/nginx.
USER node
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
