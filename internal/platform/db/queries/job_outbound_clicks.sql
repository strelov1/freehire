-- name: RecordJobOutboundClick :exec
-- Write one click. Best-effort by contract: the redirect happens whether or not this succeeds,
-- because the link lives in an email, a chat answer or an agent's output the reader cannot retry.
INSERT INTO job_outbound_clicks (
    job_id, src, is_likely_bot, device_type, os_family, ua_family, referrer_host, country, visitor_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
