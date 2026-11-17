-- Saved-search webhook deliveries become signed, like change webhooks (0268): each
-- destination gets an HMAC secret, shown to its owner in settings. Existing destinations get
-- one here (gen_random_uuid is a strong random source); new ones get theirs from the API.
ALTER TABLE public.webhook_configs ADD COLUMN IF NOT EXISTS secret TEXT;

UPDATE public.webhook_configs
SET secret = 'whsec_' || replace(gen_random_uuid()::text, '-', '') || replace(gen_random_uuid()::text, '-', '')
WHERE secret IS NULL;

-- squawk-ignore adding-not-nullable-field -- one row per webhook destination (a handful on prod): the scan is instant.
ALTER TABLE public.webhook_configs ALTER COLUMN secret SET NOT NULL;
