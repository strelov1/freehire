# Production image for the freehire web frontend (SvelteKit SSR, adapter-node).
#
# A single public container keeps everything same-origin for the SameSite=Lax
# auth cookie: nginx listens on :80 and routes
#   - /api, /health  -> the Go backend (app:8080)
#   - everything else -> the Node SSR server (127.0.0.1:3000)
# The Node server fetches the Go API server-to-server via API_INTERNAL_URL.
#
# Build context is the repo root (the sibling design-system/ package is reached
# via the file:../design-system dependency):
#   docker build -f web/Dockerfile .

# --- design-system build stage ---
# Must build before web — web's file:../design-system dependency links here.
FROM node:22-alpine AS ds-build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /design-system
COPY design-system/package.json design-system/pnpm-lock.yaml design-system/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY design-system/ ./
RUN pnpm build

# --- web build stage ---
FROM node:22-alpine AS build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /src
# The file:../design-system dependency resolves to /design-system from WORKDIR /src.
COPY --from=ds-build /design-system /design-system
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# Sentry source-map upload (optional). When SENTRY_AUTH_TOKEN is passed as a build
# arg, the sentrySvelteKit() Vite plugin uploads source maps (matched by debug-id,
# so no release coordination is needed) and production frontend stack traces are
# un-minified. Build-stage only — these do NOT persist into the runtime image below.
# Absent token ⇒ upload is skipped and the build still succeeds.
ARG SENTRY_AUTH_TOKEN=""
ARG SENTRY_ORG=""
ARG SENTRY_PROJECT=""
ENV SENTRY_AUTH_TOKEN=$SENTRY_AUTH_TOKEN
ENV SENTRY_ORG=$SENTRY_ORG
ENV SENTRY_PROJECT=$SENTRY_PROJECT
RUN pnpm run build
# Keep only production deps for the runtime image (the build tools are no longer
# needed; adapter-node has already bundled the app).
RUN pnpm prune --prod

# --- runtime stage ---
FROM node:22-alpine
RUN apk add --no-cache nginx
WORKDIR /app
COPY --from=build /src/build ./build
COPY --from=build /src/node_modules ./node_modules
COPY --from=build /src/package.json ./package.json
# The CLI install script is served as a static file by nginx (see nginx.conf).
COPY --from=build /src/public/install.sh /usr/share/nginx/html/install.sh
COPY web/nginx.conf /etc/nginx/http.d/default.conf
COPY web/docker-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Node SSR server settings. ORIGIN must be set at deploy to the public site URL
# (used for canonical/sitemap/JSON-LD); PROTOCOL_HEADER/HOST_HEADER let it derive
# the origin from nginx's forwarded headers as a fallback. See web/.env.example.
ENV PORT=3000
ENV API_INTERNAL_URL=http://app:8080
ENV PROTOCOL_HEADER=x-forwarded-proto
ENV HOST_HEADER=x-forwarded-host

EXPOSE 80
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
