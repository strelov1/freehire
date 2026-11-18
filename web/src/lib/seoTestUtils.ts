import type { Company, Job } from './types';

// Fixtures shared by seo's tests.

// collectionPageJsonLd reads only title + public_slug off each job.
export function job(title: string, slug: string): Job {
  return { title, public_slug: slug } as Job;
}

// Minimal valid Company; individual tests spread in the facts under test.
export function company(overrides: Partial<Company> = {}): Company {
  return {
    slug: 'acme',
    name: 'Acme',
    collections: [],
    created_at: null,
    updated_at: null,
    upvote_count: 0,
    downvote_count: 0,
    my_vote: 0,
    feedback_count: 0,
    feedback_rating_avg: null,
    ...overrides,
  };
}

export const ORIGIN = 'https://freehire.me';
