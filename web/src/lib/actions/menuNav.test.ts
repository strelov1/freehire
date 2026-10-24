import { describe, it, expect } from 'vitest';
import { nextMenuIndex } from './menuNav';

describe('nextMenuIndex', () => {
  it('moves down and wraps past the last item', () => {
    expect(nextMenuIndex(0, 'ArrowDown', 3)).toBe(1);
    expect(nextMenuIndex(2, 'ArrowDown', 3)).toBe(0);
  });

  it('moves up and wraps before the first item', () => {
    expect(nextMenuIndex(1, 'ArrowUp', 3)).toBe(0);
    expect(nextMenuIndex(0, 'ArrowUp', 3)).toBe(2);
  });

  it('enters from outside the items at the first (Down) or last (Up) item', () => {
    expect(nextMenuIndex(-1, 'ArrowDown', 3)).toBe(0);
    expect(nextMenuIndex(-1, 'ArrowUp', 3)).toBe(2);
  });

  it('jumps to the ends with Home/End', () => {
    expect(nextMenuIndex(1, 'Home', 3)).toBe(0);
    expect(nextMenuIndex(1, 'End', 3)).toBe(2);
  });

  it('returns null for non-navigation keys (activation is left to the browser)', () => {
    expect(nextMenuIndex(0, 'Enter', 3)).toBeNull();
    expect(nextMenuIndex(0, 'ArrowRight', 3)).toBeNull();
    expect(nextMenuIndex(0, 'a', 3)).toBeNull();
  });

  it('returns null when there are no items', () => {
    expect(nextMenuIndex(-1, 'ArrowDown', 0)).toBeNull();
  });
});
