import { isRedirect } from '@sveltejs/kit';
import { expect, it, vi } from 'vitest';

vi.mock('$app/paths', () => ({ resolve: (_route: string, params: { slug: string }) => `/jobs/${params.slug}` }));

const { load } = await import('./+page.server');

it('sends the old discussion page to the job page with the Discussion tab open', () => {
  try {
    load({ params: { slug: 'acme-job' } } as Parameters<typeof load>[0]);
  } catch (e) {
    if (!isRedirect(e)) throw e;
    expect(e.status).toBe(301);
    expect(e.location).toBe('/jobs/acme-job?tab=discussion');
    return;
  }
  throw new Error('expected a redirect');
});
