# Phase 7 — Verify

Clean-checkout verification pass. All checks run from the phase 6 branch state (which includes all phases 1-6).

## design-system

| check | result |
|---|---|
| `pnpm install --frozen-lockfile` | ✓ |
| `pnpm tokens:build` | ✓ (2 CSS files generated) |
| `pnpm validate:docs` | ✓ (23 entities across 3 JSON files) |
| `pnpm build-storybook` | ✓ (138 modules, 1.97s) |

## web

| check | result |
|---|---|
| `pnpm install --frozen-lockfile` | ✓ |
| `pnpm run check` | ✓ (0 errors, 11 pre-existing warnings) |
| `pnpm run lint` | ✓ (0 errors, 3 pre-existing warnings) |
| `pnpm run build` | ✓ (adapter-node build green) |

## Token coverage

- **design-system/src/*.svelte:** 0 hardcoded colors, 0 Tailwind arbitrary values.
- **web/src/lib/ui/:** 0 hardcoded colors, 0 Tailwind arbitrary values.

All primitives use CSS custom properties generated from the DTCG token build. No hex/rgb/oklch literals, no `bg-[#...]` or `p-[7px]` patterns.

## CI (GitHub Actions)

All CI checks pass on every phase PR:
- `design-system` job: install + build + test + validate:docs + build-storybook
- `web` job: install + lint + check + build (depends on `design-system`)
- `backend` job: go build + vet + gofmt + unit + integration tests
- `pr-smoke` job: Docker build + compose up + health check

## Inventory + DSDS docs

- `docs/component-inventory.md`: 99 components classified (29 components, 35 views, 10 layouts, 20 patterns)
- `docs/icon-inventory.md`: 172 library usages + 13 bespoke SVGs
- `docs/dsds/foundation.json`: 7 foundation entities
- `docs/dsds/theme.json`: 1 dark theme entity
- `docs/dsds/components.json`: 15 component entities

All docs checked in and validated by `pnpm validate:docs`.
