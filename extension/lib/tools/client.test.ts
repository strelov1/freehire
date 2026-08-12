import { describe, it, expect } from 'vitest';
import { respondTo } from './client';
import type { PageBridge } from './executor';
import { must } from '../test-utils';

const page: PageBridge = {
  readPage: async () => ({ url: 'https://example.test/', title: '', headline: '', text: '' }),
  readForm: async () => ({ fields: [], uploads: [] }),
  fillSimple: async (fills) => fills.map((f) => ({ label: f.label, status: 'filled' as const })),
  combobox: async () => ({ status: 'not_found' }),
};

describe('respondTo', () => {
  it('answers a tool call with a result frame correlated by id', async () => {
    const out = await respondTo('{"id":"x1","tool":"read_form"}', page);

    expect(JSON.parse(must(out))).toEqual({ id: 'x1', result: { fields: [], uploads: [] } });
  });

  it('answers a failing call with an error frame rather than nothing', async () => {
    const out = await respondTo('{"id":"x2","tool":"nope"}', page);

    expect(JSON.parse(must(out))).toMatchObject({ id: 'x2', error: expect.stringMatching(/unknown tool/i) });
  });

  it('stays silent on a frame it cannot correlate to a call', async () => {
    expect(await respondTo('garbage', page)).toBeNull();
    expect(await respondTo('{"tool":"read_form"}', page)).toBeNull();
  });
});
