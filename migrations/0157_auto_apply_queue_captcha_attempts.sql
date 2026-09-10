-- Give a captcha refusal its own attempt counter.
--
-- A captcha refusal is the one post-submit outcome safe to retry, and it needs many more
-- tries than an ordinary failure: an invisible hCaptcha is a coin toss no browser
-- configuration improves (measured 2026-09-10 over eight probe runs against a live Lever
-- posting — headless and windowed under Xvfb, datacentre and residential IP, with and
-- without warm-up: one pass). Counting those asks in `attempts` poisoned the ordinary
-- budget: a live entry reached 12 captcha refusals, and then the first genuinely transient
-- error — a field fill timing out — dead-lettered it instantly, without the three tries
-- that error was entitled to.
--
-- Same remedy migration 0140 already applied to the preview pass, for the same reason.
-- A bounded retry counter capped at 20 by the code that writes it, matching the integer
-- `attempts` and `preview_attempts` beside it — a bigint here would only make the three
-- columns disagree about the same kind of number.
ALTER TABLE auto_apply_queue
    -- squawk-ignore prefer-bigint-over-int
    ADD COLUMN captcha_attempts integer NOT NULL DEFAULT 0;
