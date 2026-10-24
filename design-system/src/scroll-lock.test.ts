import { afterEach, describe, expect, it, vi } from 'vitest';
import { lockScroll, unlockScroll } from './scroll-lock.js';

describe('scroll lock', () => {
  afterEach(() => {
    document.body.removeAttribute('style');
  });

  it('pins the body and puts back the scroll position and prior styles', () => {
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    Object.defineProperty(window, 'scrollY', { value: 300, configurable: true });
    document.body.style.overflow = 'scroll';

    lockScroll();
    expect(document.body.style.position).toBe('fixed');
    expect(document.body.style.top).toBe('-300px');
    expect(document.body.style.overflow).toBe('hidden');

    unlockScroll();
    expect(document.body.style.position).toBe('');
    expect(document.body.style.overflow).toBe('scroll');
    expect(scrollTo).toHaveBeenCalledWith(0, 300);
    scrollTo.mockRestore();
  });

  it('stays locked until every lock is released', () => {
    lockScroll();
    lockScroll();
    unlockScroll();
    expect(document.body.style.position).toBe('fixed');
    unlockScroll();
    expect(document.body.style.position).toBe('');
  });

  it('ignores an unbalanced release', () => {
    unlockScroll();
    lockScroll();
    expect(document.body.style.position).toBe('fixed');
    unlockScroll();
  });
});
