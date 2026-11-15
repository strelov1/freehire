# Scoped source search

Run `python3 scripts/source-search.py SCOPE PATTERN` from the repository root.
Choose the smallest owning scope first; widen explicitly if ownership is uncertain.
The helper uses ripgrep's normal ignore rules and omits instruction references,
dependencies, generated SQL query implementations and API code, lockfiles and archived
OpenSpec changes. Live application code in a directory named `history` remains visible.
It changes no global or repository ignore settings.

Use `--full` to search hidden, ignored, generated and history files too.
SQL queries, migrations, TypeSpec and other generation inputs stay visible by default.
Ripgrep exit codes are preserved: 0 matches, 1 no matches, 2 error.

Examples:

```sh
python3 scripts/source-search.py internal/api/handler paidAI
python3 scripts/source-search.py internal/platform/db/queries UpsertJob
python3 scripts/source-search.py internal/ai WithPaidAI
python3 scripts/source-search.py web/src EntityLogo
python3 scripts/source-search.py cmd DATABASE_URL
python3 scripts/source-search.py --full internal/platform/db UpsertJob
```
