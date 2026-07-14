import { describe, it, expect } from 'vitest';
import { nextTabIndex } from './tablist';

describe('nextTabIndex', () => {
  it('moves right and wraps past the last tab', () => {
    expect(nextTabIndex(0, 'ArrowRight', 3)).toBe(1);
    expect(nextTabIndex(2, 'ArrowRight', 3)).toBe(0);
  });

  it('moves left and wraps before the first tab', () => {
    expect(nextTabIndex(1, 'ArrowLeft', 3)).toBe(0);
    expect(nextTabIndex(0, 'ArrowLeft', 3)).toBe(2);
  });

  it('jumps to the ends with Home/End', () => {
    expect(nextTabIndex(1, 'Home', 3)).toBe(0);
    expect(nextTabIndex(1, 'End', 3)).toBe(2);
  });

  it('returns null for non-navigation keys (manual activation is left to the browser)', () => {
    expect(nextTabIndex(0, 'Enter', 3)).toBeNull();
    expect(nextTabIndex(0, ' ', 3)).toBeNull();
    expect(nextTabIndex(0, 'a', 3)).toBeNull();
  });

  it('returns null when there are no tabs', () => {
    expect(nextTabIndex(0, 'ArrowRight', 0)).toBeNull();
  });
});
