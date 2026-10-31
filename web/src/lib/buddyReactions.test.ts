import { describe, expect, it } from 'vitest';
import { applyReaction, defaultReaction, reactionSummary } from './buddyReactions';

describe('defaultReaction', () => {
  it('offers the reaction that fits the news', () => {
    expect(defaultReaction('offer')).toBe('congrats');
    expect(defaultReaction('interview')).toBe('luck');
    expect(defaultReaction('applied')).toBe('rooting');
  });
});

describe('reactionSummary', () => {
  it('names the most chosen kinds first, at most three, with the total', () => {
    expect(reactionSummary({ congrats: 5, luck: 1, same: 3, insight: 2 })).toEqual({ top: ['congrats', 'same', 'insight'], total: 11 });
  });
  it('is empty when nobody reacted', () => {
    expect(reactionSummary({})).toEqual({ top: [], total: 0 });
  });
});

describe('applyReaction', () => {
  it('adds the viewer’s first reaction', () => {
    expect(applyReaction({ counts: {}, mine: '' }, 'luck')).toEqual({ counts: { luck: 1 }, mine: 'luck' });
  });
  it('moves the viewer from one kind to another, dropping a kind that reaches zero', () => {
    expect(applyReaction({ counts: { luck: 1, congrats: 2 }, mine: 'luck' }, 'congrats')).toEqual({ counts: { congrats: 3 }, mine: 'congrats' });
  });
  it('clears the viewer’s reaction', () => {
    expect(applyReaction({ counts: { luck: 2 }, mine: 'luck' }, '')).toEqual({ counts: { luck: 1 }, mine: '' });
  });
});
