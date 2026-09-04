# internal/dict

The facet dictionaries and the normalisation rules under them — skills, locations, seniority, industry, role, language, company names. Dict-only in production: a value that is not in the dictionary yields nothing rather than a guess.

**Layer 2 of 8.**

May import: `platform` — and itself.

Must NOT import: `ai`, `identity`, `candidate`, `job`, `application`, `search`, `engage`, `ingest`, `api`.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`classify` `companyname` `industrytag` `lang` `location` `normalize` `roletype` `skilladjacency` `skillbundle` `skilltag` `vocab` `wordmatch`
