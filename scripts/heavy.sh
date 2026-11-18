#!/bin/sh
# Runs a heavy command (tests, type checks, linters, builds) in a machine-wide slot of heavy
# (https://github.com/mokevnin/heavy), so parallel checkouts and agents take turns instead of
# starving the machine. Without heavy (an outside contributor, CI) it runs the command directly.
# A busy machine slows a check down after 5 minutes of waiting but never fails it.
if command -v heavy >/dev/null 2>&1; then
  exec heavy --timeout 300 --on-timeout run -- "$@"
fi
exec "$@"
