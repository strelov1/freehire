import { describe, it, expect } from 'vitest';
import { STATIC_PATHS, collectionPaths } from './sitemap';

describe('sitemap static paths', () => {
  it('includes the collections hub and the for-companies page', () => {
    expect(STATIC_PATHS).toContain('/collections');
    expect(STATIC_PATHS).toContain('/for-companies');
  });
});

describe('collectionPaths', () => {
  it('maps every collection slug to its /collections/:slug landing path', () => {
    const paths = collectionPaths();
    expect(paths).toContain('/collections/yc'); // company collection
    expect(paths).toContain('/collections/remote-worldwide'); // filter collection
    expect(new Set(paths).size).toBe(paths.length);
  });
});
