-- The seat rule now leaves a one-word name its seat, so "KRKA, d.d., Novo mesto" derives
-- `krka-d-d-novo-mesto` again and ingest looks it up under the fold `krkaddnovomesto`. The alias
-- row was written while the rule keyed that name at `krka`, under the fold `krka`, so without
-- this the employer's next posting of that spelling would open the duplicate again.
--
-- Safe to run before the binary: under the old rule the name derives `krka` itself and needs no
-- alias. A database without this row updates nothing.
UPDATE company_slug_aliases
SET folded_key = 'krkaddnovomesto'
WHERE alias_slug = 'krka-d-d-novo-mesto' AND canonical_slug = 'krka' AND folded_key = 'krka';
