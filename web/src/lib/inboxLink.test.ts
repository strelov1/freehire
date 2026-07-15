import { describe, it, expect } from 'vitest';
import { inboxLinkState, type LastUnlinked } from './inboxLink';

const unlinked = { id: 1 };
const linked = { id: 1, linked_slug: 'acme' };
const suggested = { id: 1, suggested_slug: 'acme' };
const stash: LastUnlinked = { id: 1, slug: 'acme', company: 'Acme' };

describe('inboxLinkState', () => {
	it('is "linked" when the email has a linked application', () => {
		expect(inboxLinkState(linked, null)).toBe('linked');
	});

	it('is "suggested" when the email carries a pending suggestion', () => {
		expect(inboxLinkState(suggested, null)).toBe('suggested');
	});

	it('is "undo" when an unlinked email matches the last unlink', () => {
		expect(inboxLinkState(unlinked, stash)).toBe('undo');
	});

	it('is "picker" for an unlinked email with no matching last unlink', () => {
		expect(inboxLinkState(unlinked, null)).toBe('picker');
	});

	it('does not offer undo for a different email than the one unlinked', () => {
		expect(inboxLinkState({ id: 2 }, stash)).toBe('picker');
	});

	it('prefers the real link over a stale last-unlink stash', () => {
		expect(inboxLinkState(linked, stash)).toBe('linked');
	});
});
