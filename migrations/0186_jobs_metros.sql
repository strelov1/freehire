-- Metro areas (GHS-FUA functional urban areas) a posting's location falls in,
-- "Sacramento (US)". Derived by internal/dict/location like cities; see
-- docs/superpowers/specs/2026-10-21-metro-areas-design.md.
-- A constant default is a metadata-only change on PG 11+: no table rewrite.
ALTER TABLE jobs ADD COLUMN metros text[] DEFAULT '{}'::text[] NOT NULL;
