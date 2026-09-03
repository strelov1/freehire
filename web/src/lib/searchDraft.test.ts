import { describe, it, expect } from 'vitest';
import { emptyDraft, edit, reconcile, commit } from './searchDraft';

// The header input stops filtering the list on every keystroke: what you type is a
// DRAFT, and only Enter or choosing a suggestion commits it. That splits one string
// into two, and the whole difficulty is keeping them honest when the committed query
// moves on its own — back/forward, a filter chip removed, a suggestion applied
// elsewhere. These cover that reconciliation at its pure core; the Svelte component
// holds the same shape in `$state` (which this test env has no runtime for, same
// reason paginated.svelte.test.ts tests `loadWithRetry` rather than `Paginator`).

describe('searchDraft', () => {
  it('starts showing the committed query', () => {
    expect(emptyDraft('nodejs').text).toBe('nodejs');
  });

  it('holds typing without committing it', () => {
    const typed = edit(emptyDraft(''), 'java dev');
    expect(typed.text).toBe('java dev');
    expect(typed.committed).toBe('');
  });

  it('commits the typed text on demand', () => {
    const typed = edit(emptyDraft(''), 'java dev');
    expect(commit(typed).committed).toBe('java dev');
  });

  it('trims on commit so a stray space is not a different query', () => {
    expect(commit(edit(emptyDraft(''), '  java dev  ')).committed).toBe('java dev');
  });

  // The reconciliation. An external move must reach the box; the visitor's own typing
  // must not be overwritten by the stale value it has not committed yet.
  it('follows the committed query when it moves externally', () => {
    const typed = edit(emptyDraft('java'), 'java dev');
    expect(reconcile(typed, 'python').text).toBe('python');
  });

  it('leaves uncommitted typing alone while the committed query is unchanged', () => {
    const typed = edit(emptyDraft('java'), 'java dev');
    expect(reconcile(typed, 'java').text).toBe('java dev');
  });

  it('does not bounce back to the old text after its own commit', () => {
    const committed = commit(edit(emptyDraft('java'), 'java dev'));
    expect(reconcile(committed, 'java dev').text).toBe('java dev');
  });

  it('clears the box when the committed query is cleared externally', () => {
    const typed = edit(emptyDraft('java'), 'java dev');
    expect(reconcile(typed, '').text).toBe('');
  });

  it('commits whitespace-only text as empty — the clear-by-spacebar path', () => {
    expect(commit(edit(emptyDraft('java'), '   ')).committed).toBe('');
  });

  // The scope. A draft belongs to the list it was typed into, and TopBar keeps ONE
  // HeaderListSearch across `/`, `/companies/:slug` and `/collections/:slug` — so the
  // component survives a navigation between two different lists. Both carry `q=''`,
  // which the committed rule alone reads as "no news", leaving the previous page's
  // text over the new page's results.
  describe('scope', () => {
    const listA = { name: 'a' };
    const listB = { name: 'b' };

    it('drops the draft when a different list takes over the box', () => {
      const typed = edit(emptyDraft('', listA), 'java dev');
      expect(reconcile(typed, '', listB).text).toBe('');
    });

    it('keeps the draft while the same list owns the box', () => {
      const typed = edit(emptyDraft('', listA), 'java dev');
      expect(reconcile(typed, '', listA).text).toBe('java dev');
    });

    // The view registers its store ~300ms after first paint, so the box exists
    // unowned before that. Text typed into it is the visitor's, not a previous
    // list's, and adopting an owner must not throw it away.
    it('adopts a first owner without discarding what was typed meanwhile', () => {
      const typed = edit(emptyDraft(''), 'java dev');
      const owned = reconcile(typed, '', listA);
      expect(owned.text).toBe('java dev');
      expect(reconcile(owned, '', listB).text).toBe('');
    });
  });
});
