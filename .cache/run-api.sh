#!/bin/sh
# Local API for eyeballing the /mentors screens. Untracked (.cache/), not a deploy path.
set -eu

export DATABASE_URL="postgres://hire:hire@127.0.0.1:5442/hire?sslmode=disable"
export REDIS_URL="redis://192.168.97.3:6379/0"
# Length is the only thing the config checks, and it refuses anything under 32 bytes.
export JWT_SECRET="local-development-only-not-a-secret-000000"
export FRONTEND_ORIGIN="http://localhost:5173"
export PORT="8080"

exec go run ./cmd/server
