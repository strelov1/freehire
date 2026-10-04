# internal/identity

Who the caller is: accounts, the auth primitives and providers, profile, account deletion.

**Layer 3 of 8.**

May import: `platform`, `dict` — and itself.

Must NOT import: `ai`, `candidate`, `job`, `application`, `search`, `engage`, `ingest`, `api`.

`ai` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`accountdelete` `accounts` `auth` `auth/apple` `auth/applejobs` `auth/mobileauth` `auth/oauth` `auth/oauth2server` `auth/recentauth` `userprofile`
