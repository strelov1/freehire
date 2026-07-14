// Generates the static brand Open Graph image committed at web/static/og.png.
//
// The brand card is essentially fixed (headline + baked-in catalogue figures), so
// it is a committed static asset rather than an on-demand endpoint — zero runtime
// cost, and it is the default og:image for every page without its own preview. This
// script renders it through the real satori render core (shared bootstrap in
// ./og-render.mjs), asserts the result is a valid PNG, and writes it. Re-run and
// re-commit when the catalogue figures grow materially.
//
//   node scripts/gen-og.mjs        # writes web/static/og.png; exits non-zero on failure
//
// Stat figures mirror the homepage fallbacks in src/lib/components/HomeView.svelte.

import { writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { createOgVite, isValidPng, loadFonts, webRoot } from './og-render.mjs';

const outPath = resolve(webRoot, 'static/og.png');

const STATS = [
  { value: '3.1M+', label: 'open jobs' },
  { value: '220K+', label: 'companies' },
  { value: '98', label: 'ATS platforms' },
];

async function main() {
  const vite = await createOgVite();

  try {
    const { renderMarkupPng } = await vite.ssrLoadModule('/src/lib/server/og/render.ts');
    const { buildBrandCard } = await vite.ssrLoadModule('/src/lib/server/og/brand.ts');
    const fonts = await loadFonts();

    const png = Buffer.from(await renderMarkupPng(buildBrandCard({ stats: STATS }), fonts));
    if (!isValidPng(png)) {
      console.error(`Invalid PNG (${png.length} bytes)`);
      process.exit(1);
    }

    await writeFile(outPath, png);
    console.log(`Wrote ${outPath} (${png.length} bytes)`);
  } finally {
    await vite.close();
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
