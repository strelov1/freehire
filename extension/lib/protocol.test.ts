import { describe, it, expect } from 'vitest';
import { emptySnapshot } from './protocol';

describe('emptySnapshot', () => {
  it('is all-empty', () => {
    expect(emptySnapshot()).toEqual({ url: '', title: '', headline: '', text: '' });
  });
});
