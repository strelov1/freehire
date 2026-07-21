import { describe, it, expect } from 'vitest';
import { nextTrapIndex } from './focusTrap';

describe('nextTrapIndex', () => {
  it('pulls escaped focus (current -1) back to the first item', () => {
    expect(nextTrapIndex(-1, 3, false)).toBe(0);
    expect(nextTrapIndex(-1, 3, true)).toBe(0);
  });

  it('wraps forward Tab from the last item to the first', () => {
    expect(nextTrapIndex(2, 3, false)).toBe(0);
  });

  it('wraps backward Shift+Tab from the first item to the last', () => {
    expect(nextTrapIndex(0, 3, true)).toBe(2);
  });

  it('does not intervene on normal in-bounds tabbing', () => {
    expect(nextTrapIndex(1, 3, false)).toBeNull(); // middle forward
    expect(nextTrapIndex(1, 3, true)).toBeNull(); // middle backward
    expect(nextTrapIndex(0, 3, false)).toBeNull(); // first forward → browser handles
    expect(nextTrapIndex(2, 3, true)).toBeNull(); // last backward → browser handles
  });

  it('no-ops with no focusable items', () => {
    expect(nextTrapIndex(-1, 0, false)).toBeNull();
    expect(nextTrapIndex(0, 0, true)).toBeNull();
  });
});
