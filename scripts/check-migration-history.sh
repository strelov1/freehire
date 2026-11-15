#!/bin/sh
# Local upgrades must not silently baseline pending files on an old initdb volume.
set -eu
psql -X --dbname="${DATABASE_URL:?DATABASE_URL is required; run make through mise exec}" \
  --set=ON_ERROR_STOP=1 --quiet <<'SQL'
DO $$
DECLARE
  recorded boolean := false;
BEGIN
  IF to_regclass('public.jobs') IS NULL THEN
    RETURN;
  END IF;
  IF to_regclass('public.schema_migrations') IS NOT NULL THEN
    EXECUTE 'SELECT EXISTS (SELECT 1 FROM public.schema_migrations)' INTO recorded;
  END IF;
  IF NOT recorded THEN
    RAISE EXCEPTION 'Existing schema has no migration history. Baseline only the files known to have initialized this database before upgrading; see README.md (Local development).';
  END IF;
END
$$;
SQL
