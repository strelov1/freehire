#!/usr/bin/env bash
# One-shot: wait for the running semantic reindex (PID 2684382) to finish, then run the
# facet reindex to completion (populates react_native/flutter/team_lead across the
# whole catalogue), then restore the normal 3-hourly facet timer. Installed
# 2026-07-07 after a green deploy aborted the in-flight facet reindex; user chose
# semantic-first to avoid the Meili facet/semantic stacking deadlock.
set -uo pipefail
while kill -0 2684382 2>/dev/null; do sleep 60; done
sleep 120  # let Meili finalize the semantic rebuild swap
logger -t after-semantic-reindex "semantic PID 2684382 gone; starting facet reindex"
systemctl start freehire-reindexw.service
systemctl start freehire-reindexw.timer
logger -t after-semantic-reindex "facet reindex kicked; timer restored"
