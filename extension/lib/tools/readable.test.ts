import { describe, it, expect } from 'vitest';
import { isReadablePageUrl } from './readable';

describe('isReadablePageUrl', () => {
  it('admits ordinary web pages', () => {
    expect(isReadablePageUrl('https://boards.greenhouse.io/acme/jobs/12')).toBe(true);
    expect(isReadablePageUrl('http://localhost:5173/jobs')).toBe(true);
  });

  // The agent's reach should stop at the web. None of these are postings, and two
  // of them are the user's own machine.
  it('refuses everything that is not the web', () => {
    for (const url of [
      'chrome://settings/passwords',
      'chrome-extension://abcdef/popup.html',
      'file:///Users/someone/taxes.pdf',
      'about:blank',
      'edge://settings',
      'view-source:https://example.test/',
    ]) {
      expect(isReadablePageUrl(url), url).toBe(false);
    }
  });

  // A tab with no url yet, or something we cannot parse, is not a page to read.
  it('refuses what it cannot make sense of', () => {
    expect(isReadablePageUrl('')).toBe(false);
    expect(isReadablePageUrl('not a url')).toBe(false);
    expect(isReadablePageUrl(undefined)).toBe(false);
  });
});
