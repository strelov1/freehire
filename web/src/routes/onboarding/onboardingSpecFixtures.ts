// Fixtures shared by the /onboarding page specs (page.spec.ts, page.saving.spec.ts):
// a parsed CV, the top matches, and the steps that drop a CV and answer the questions.
import { fireEvent, screen } from '@testing-library/svelte';

import type { Job, OnboardingParse } from '#lib/types.js';

export const parse: OnboardingParse = {
  structured_available: true,
  first_name: 'Ana',
  headline: 'Backend engineer',
  country: 'br',
  total_years: 9,
  companies: ['Nubank', 'Stone'],
  skills: ['go', 'kubernetes'],
  categories: ['backend'],
  seniority: 'senior',
  report: {
    overall: 64,
    potential: 88,
    reviewed: false,
    strong_keywords: [],
    recommended_keywords: [],
    categories: [
      {
        id: 'impact',
        label: 'Impact',
        score: 5,
        max: 20,
        items: [{ points: 0, text: 'Quantify results', status: 'fail' }],
      },
    ],
  },
};

export const job = (n: number, company = 'Acme') =>
  ({
    public_slug: `job-${n}`,
    title: `Platform Engineer ${n}`,
    company,
    work_mode: 'remote',
    countries: ['br'],
    skills: ['go', 'kubernetes'],
  }) as unknown as Job;

// Seven top matches at five employers, out of 526 the search found.
const COMPANIES = ['Magnit Tech', 'Placements24', 'Magnit Tech', 'Acme', 'Globex', 'Initech', 'Umbrella'];
export const topMatches = COMPANIES.map((c, i) => ({ job: job(i + 1, c), match_percent: 92 - i }));

const pdf = () => new File(['%PDF'], 'cv.pdf', { type: 'application/pdf' });

export async function dropCv() {
  const input = document.querySelector('input[type=file]') as HTMLInputElement;
  await fireEvent.change(input, { target: { files: [pdf()] } });
}

export async function answerAll(challenges: string[] = []) {
  await fireEvent.click(await screen.findByRole('button', { name: 'Remote' }));
  await fireEvent.click(await screen.findByRole('button', { name: /^Senior Backend/ }));
  await screen.findByRole('heading', { name: "What's in your way right now?" });
  for (const label of challenges) await fireEvent.click(screen.getByRole('button', { name: label }));
  await fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
}
