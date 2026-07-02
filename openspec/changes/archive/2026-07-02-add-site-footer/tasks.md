## 1. Footer component

- [x] 1.1 Create `web/src/lib/components/Footer.svelte` scaffold: `<footer>` with the
  responsive grid shell and a brand block (project name + one-line tagline)
- [x] 1.2 Add the social links (GitHub, LinkedIn, Telegram) to the brand block, each
  via `ProviderIcon` with `target="_blank" rel="noopener noreferrer"`; LinkedIn →
  `https://linkedin.com/company/freehire-dev/`
- [x] 1.3 Add the navigation columns — Product (Jobs, Companies, Collections,
  Recruiters), Resources (CLI, API docs), Company (For companies, Submit) — using
  `resolve()` for internal links
- [x] 1.4 Add the bottom bar: copyright line + open-source note, separated by a thin
  top border; confirm responsive stacking and light/dark tokens

## 2. Integration

- [x] 2.1 Replace the inline `<footer>` block in `web/src/routes/+layout.svelte` with
  `<Footer />` (import from `$lib/components/Footer.svelte`)

## 3. Verify

- [x] 3.1 `svelte-check` passes for the new component and the layout; lint shows no new
  errors
- [x] 3.2 Visual check: footer renders correctly in light and dark themes and at a
  mobile viewport width (columns stack, no horizontal overflow)
