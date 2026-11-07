import { describe, expect, it, vi } from 'vitest';

vi.mock('$app/paths', () => ({ resolve: (route: string, params: Record<string, string>) => route.replace('[slug]', params.slug ?? '') }));

import { tailorHref } from './tailorHref';

describe('tailorHref', () => {
  it('points at the workspace for a vacancy', () => {
    expect(tailorHref('senior-go-acme-1a2b')).toBe('/tailor/senior-go-acme-1a2b');
  });

  it('carries a chosen base in the URL, because the workspace re-runs its request on reload', () => {
    expect(tailorHref('senior-go-acme-1a2b', '0f2c')).toBe('/tailor/senior-go-acme-1a2b?base=0f2c');
  });

  it('leaves the URL bare when no base was chosen', () => {
    expect(tailorHref('x', undefined)).toBe('/tailor/x');
    expect(tailorHref('x', '')).toBe('/tailor/x');
  });
});
