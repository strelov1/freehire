#!/bin/sh
# Run the SvelteKit Node SSR server in the background and nginx in the foreground.
# nginx is the public face (:8080); it proxies non-API routes to the Node server on
# 127.0.0.1:3000 and /api,/health to the Go backend. If nginx exits, the
# container exits; the orchestrator restarts it (and a stalled Node surfaces as
# 502s on a healthcheck).
set -e

# Variable proxy_pass needs a recursive resolver. nginx.conf defaults to Docker's
# embedded DNS (127.0.0.11); Podman/aardvark put a different nameserver in
# /etc/resolv.conf — without this swap, /api/* returns 502.
RESOLVER=$(awk '/^nameserver / { print $2; exit }' /etc/resolv.conf)
RESOLVER=${RESOLVER:-127.0.0.11}
mkdir -p /tmp/freehire-web "$HOME"
# Render into scratch: the template and the rest of the image can remain read-only.
sed "s/resolver 127\\.0\\.0\\.11 /resolver ${RESOLVER} /" /etc/nginx/http.d/default.conf > /tmp/freehire-web/default.conf

node build &

exec nginx -e stderr -g 'daemon off;'
