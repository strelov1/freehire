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

import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { createOgVite, isValidPng, loadFonts } from './og-render.mjs';

const outDir = '/tmp/og-smoke';

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

// A company entity; per-fixture overrides tweak the fields the company card reads.
function company(overrides = {}) {
  return {
    slug: 'smoke',
    name: 'Acme Corp',
    collections: [],
    created_at: null,
    updated_at: null,
    ...overrides,
  };
}

// [entity, openJobs] pairs covering the degradation cases the company card handles.
const companyFixtures = {
  companyFull: [
    company({
      name: 'Supabase',
      tagline: 'The open source Firebase alternative',
      industries: ['Developer Tools'],
      hq_country: 'US',
    }),
    128,
  ],
  companyNoChips: [company({ name: 'Zzz Obscure Holdings' }), 3],
  companyOneJob: [company({ name: 'Solo Studio', hq_country: 'DE' }), 1],
  companyZeroJobs: [company({ name: 'Dormant Inc', tagline: 'Nothing open right now' }), 0],
};

async function main() {
  const vite = await createOgVite();

  let failed = 0;
  try {
    const { renderCardPng, renderMarkupPng } = await vite.ssrLoadModule('/src/lib/server/og/render.ts');
    const { buildCompanyCard } = await vite.ssrLoadModule('/src/lib/server/og/company.ts');
    const fonts = await loadFonts();
    await mkdir(outDir, { recursive: true });

    const assertPng = async (name, png) => {
      const ok = isValidPng(png);
      if (!ok) failed++;
      await writeFile(resolve(outDir, `${name}.png`), png);
      console.log(`${ok ? 'PASS' : 'FAIL'}  ${name.padEnd(14)} ${png.length} bytes  -> ${outDir}/${name}.png`);
    };

    for (const [name, fixture] of Object.entries(fixtures)) {
      await assertPng(name, Buffer.from(await renderCardPng(fixture, { fonts, logo: null })));
    }

    for (const [name, [entity, openJobs]] of Object.entries(companyFixtures)) {
      const markup = buildCompanyCard(entity, { logo: null, openJobs });
      await assertPng(name, Buffer.from(await renderMarkupPng(markup, fonts)));
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
