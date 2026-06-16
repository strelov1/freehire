// Dependency-free render smoke test for the job OG-image pipeline.
//
// The project has no JS unit runner (and we deliberately do not add one — see the
// change's design doc). This script is the automated RED→GREEN check for the
// render core: it builds fixture jobs covering the degradation cases, runs them
// through the real renderCardPng, and asserts each result is a valid PNG.
//
// It loads the TypeScript source through Vite's SSR loader (Vite is already a
// devDependency) so the source stays in the project's normal extensionless / $lib
// style — no .ts-extension imports, no tsconfig changes, no new framework.
//
//   node scripts/og-smoke.mjs        # asserts; exits non-zero on failure
//
// Rendered PNGs are written to /tmp/og-smoke/ for visual inspection.

import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { createServer } from 'vite';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '..');
const fontsDir = resolve(webRoot, 'src/lib/server/og/fonts');
const outDir = '/tmp/og-smoke';

const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

// A complete-enough Job; per-fixture overrides tweak the fields the card reads.
function job(overrides = {}) {
  const { enrichment = {}, ...rest } = overrides;
  return {
    public_slug: 'smoke',
    source: 'smoke',
    manually_added: false,
    external_id: 'smoke',
    url: 'https://example.com',
    title: 'Software Engineer',
    company: 'Acme Corp',
    company_slug: 'acme-corp',
    location: 'Remote',
    description: '',
    countries: [],
    regions: [],
    skills: [],
    enrichment: { ...enrichment },
    enrichment_version: 1,
    ...rest,
  };
}

const fixtures = {
  full: job({
    title: 'Senior Backend Engineer',
    company: 'Supabase',
    work_mode: 'remote',
    skills: ['go', 'postgres', 'kubernetes', 'docker', 'grpc'],
    enrichment: {
      seniority: 'senior',
      salary_min: 140000,
      salary_max: 180000,
      salary_currency: 'USD',
      salary_period: 'year',
    },
  }),
  noSalary: job({
    title: 'Frontend Engineer, Design Systems',
    company: 'Vercel',
    work_mode: 'hybrid',
    skills: ['typescript', 'react'],
    enrichment: { seniority: 'middle' },
  }),
  noLogo: job({
    title: 'Product Manager',
    company: 'Zzz Obscure Holdings',
    work_mode: 'remote',
  }),
  longTitle: job({
    title:
      'Senior Machine Learning Engineer — Large-Scale Distributed Training and Inference Platform for Foundation Models',
    company: 'Nebius',
    work_mode: 'remote',
    skills: ['python', 'pytorch', 'cuda', 'kubernetes', 'ray'],
    enrichment: { seniority: 'senior', salary_min: 200000, salary_currency: 'USD', salary_period: 'year' },
  }),
};

async function loadFonts() {
  const files = [
    ['Inter-Regular.ttf', 400],
    ['Inter-SemiBold.ttf', 600],
    ['Inter-Bold.ttf', 700],
  ];
  return Promise.all(
    files.map(async ([file, weight]) => ({
      name: 'Inter',
      data: await readFile(resolve(fontsDir, file)),
      weight,
      style: 'normal',
    })),
  );
}

async function main() {
  const vite = await createServer({
    configFile: false,
    root: webRoot,
    logLevel: 'error',
    resolve: { alias: { $lib: resolve(webRoot, 'src/lib') } },
    ssr: { external: ['@resvg/resvg-js'] },
  });

  let failed = 0;
  try {
    const { renderCardPng } = await vite.ssrLoadModule('/src/lib/server/og/render.ts');
    const fonts = await loadFonts();
    await mkdir(outDir, { recursive: true });

    for (const [name, fixture] of Object.entries(fixtures)) {
      const png = Buffer.from(await renderCardPng(fixture, { fonts, logo: null }));
      const okSig = png.subarray(0, 8).equals(PNG_SIGNATURE);
      const okSize = png.length > 2000;
      const ok = okSig && okSize;
      if (!ok) failed++;
      await writeFile(resolve(outDir, `${name}.png`), png);
      console.log(
        `${ok ? 'PASS' : 'FAIL'}  ${name.padEnd(10)} ${png.length} bytes  sig=${okSig} size=${okSize}  -> ${outDir}/${name}.png`,
      );
    }
  } finally {
    await vite.close();
  }

  if (failed) {
    console.error(`\n${failed} fixture(s) failed`);
    process.exit(1);
  }
  console.log('\nAll fixtures rendered valid PNGs.');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
