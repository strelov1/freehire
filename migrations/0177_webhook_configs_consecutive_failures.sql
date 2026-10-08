-- A 410 Gone auto-disables a webhook destination (migration 0132), but any
-- other failure — a 404, 500, or timeout — only burns the claimed match's own
-- attempt budget (subscription_matches.attempts) before dead-lettering. A new
-- match for the same broken destination starts that budget over, so a
-- destination that is simply dead (wrong URL, service gone, no 410 in sight)
-- fails forever instead of being disabled once, one per notify pass (freehire
-- incident 2026-10-08: a single stale n8n webhook failed ~24 passes in a row).
--
-- consecutive_failures counts failures since the last success or re-enable;
-- RecordWebhookDeliveryFailure disables the destination once it crosses the
-- configured threshold, the same way a 410 already does.
ALTER TABLE public.webhook_configs
    ADD COLUMN IF NOT EXISTS consecutive_failures bigint NOT NULL DEFAULT 0;
