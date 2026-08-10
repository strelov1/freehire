import { describe, it, expect } from 'vitest';
import { executeTool, mergeComboboxReplies, mergeFrameOutcomes, type PageBridge } from './executor';
import { parseToolCall } from './wire';
import type { FramedField } from './wire';
import type { ComboboxStep } from '../protocol';

const field = (label: string, frame = 0): FramedField => ({
  index: 0,
  form: 0,
  frame,
  tag: 'input',
  type: 'text',
  label,
  name: '',
  required: false,
  value: '',
  combo: false,
});

function bridge(over: Partial<PageBridge> = {}): PageBridge {
  return {
    readPage: async () => ({ url: 'https://example.test/', title: '', headline: '', text: '' }),
    readForm: async () => ({ fields: [field('Email')], uploads: [] }),
    fillSimple: async (fills) => fills.map((f) => ({ label: f.label, status: 'filled' as const })),
    combobox: async () => ({ status: 'not_found' as const }),
    ...over,
  };
}

describe('executeTool', () => {
  it('answers read_form with the page fields, tagged by frame', async () => {
    const page = bridge({
      readForm: async () => ({
        fields: [field('Email'), field('Phone', 3)],
        uploads: [{ frame: 3, form: 0 }],
      }),
    });

    const res = await executeTool({ id: 'c1', tool: 'read_form' }, page);

    expect(res.id).toBe('c1');
    expect(res.error).toBeUndefined();
    // The uploads travel with the fields: they are what says the page is showing
    // an application at all, and the harness has no other way to see them.
    expect(res.result).toEqual({
      fields: [field('Email'), field('Phone', 3)],
      uploads: [{ frame: 3, form: 0 }],
    });
  });

  it('answers read_page with what the tab is showing', async () => {
    const page = bridge({
      readPage: async () => ({
        url: 'https://jobs.example.test/senior-go',
        title: 'Senior Go Engineer — Example',
        headline: 'Senior Go Engineer',
        text: 'We run a large Go fleet and are hiring.',
      }),
    });

    const res = await executeTool({ id: 'p1', tool: 'read_page' }, page);

    expect(res.id).toBe('p1');
    expect(res.error).toBeUndefined();
    expect(res.result).toEqual({
      url: 'https://jobs.example.test/senior-go',
      title: 'Senior Go Engineer — Example',
      headline: 'Senior Go Engineer',
      text: 'We run a large Go fleet and are hiring.',
    });
  });

  // The assistant is blocked on this call's id: a page it cannot read has to come
  // back as an answer, because silence is indistinguishable from a hang.
  it('answers read_page with an error when the tab cannot be reached', async () => {
    const page = bridge({
      readPage: async () => {
        throw new Error('could not reach the page');
      },
    });

    const res = await executeTool({ id: 'p2', tool: 'read_page' }, page);

    expect(res.id).toBe('p2');
    expect(res.result).toBeUndefined();
    expect(res.error).toBe('could not reach the page');
  });

  it('answers fill_simple with an outcome per requested fill', async () => {
    const res = await executeTool(
      { id: 'c2', tool: 'fill_simple', args: { fills: [{ label: 'Email', value: 'a@b.c' }] } },
      bridge(),
    );

    expect(res).toEqual({ id: 'c2', result: { outcomes: [{ label: 'Email', status: 'filled' }] } });
  });

  it('rejects fill_simple whose args carry no fills', async () => {
    const res = await executeTool({ id: 'c3', tool: 'fill_simple', args: {} }, bridge());

    expect(res.id).toBe('c3');
    expect(res.error).toMatch(/fills/);
  });

  it('reports an unknown tool as an error result, not silence', async () => {
    const res = await executeTool({ id: 'c4', tool: 'submit_form' }, bridge());

    expect(res.id).toBe('c4');
    expect(res.error).toMatch(/unknown tool/i);
  });

  it('drives the widget through the four combobox primitives', async () => {
    const seen: ComboboxStep[] = [];
    const page = bridge({
      combobox: async (step) => {
        seen.push(step);
        return { status: 'open', options: ['Yes', 'No'] };
      },
    });

    const res = await executeTool({ id: 'c6', tool: 'combobox.options', args: { label: 'Sponsorship' } }, page);

    expect(seen).toEqual([{ action: 'options', label: 'Sponsorship', value: '' }]);
    expect(res).toEqual({ id: 'c6', result: { status: 'open', options: ['Yes', 'No'] } });
  });

  it('carries the chosen option to combobox.select and combobox.verify', async () => {
    const seen: ComboboxStep[] = [];
    const page = bridge({
      combobox: async (step) => {
        seen.push(step);
        return { status: 'selected' };
      },
    });

    await executeTool({ id: 'c7', tool: 'combobox.select', args: { label: 'Country', value: 'Germany' } }, page);
    await executeTool({ id: 'c8', tool: 'combobox.verify', args: { label: 'Country', value: 'Germany' } }, page);

    expect(seen).toEqual([
      { action: 'select', label: 'Country', value: 'Germany' },
      { action: 'verify', label: 'Country', value: 'Germany' },
    ]);
  });

  it('rejects a combobox call that names no widget', async () => {
    const res = await executeTool({ id: 'c9', tool: 'combobox.open', args: {} }, bridge());

    expect(res.error).toMatch(/label/);
  });

  it('rejects a select that names no option, rather than committing a blank', async () => {
    const res = await executeTool({ id: 'c10', tool: 'combobox.select', args: { label: 'Country' } }, bridge());

    expect(res.error).toMatch(/value/);
  });
});

describe('mergeComboboxReplies', () => {
  it('keeps the frame that holds the widget over the frames that do not', () => {
    expect(
      mergeComboboxReplies([{ status: 'not_found' }, { status: 'opened' }, { status: 'not_found' }]),
    ).toEqual({ status: 'opened' });
  });

  it('reports not-found when no frame holds the widget', () => {
    expect(mergeComboboxReplies([{ status: 'not_found' }, { status: 'not_found' }])).toEqual({
      status: 'not_found',
    });
  });

  it('reports not-found when no frame answered at all', () => {
    expect(mergeComboboxReplies([])).toEqual({ status: 'not_found' });
  });

  it('turns a thrown executor failure into an error result tagged with the call id', async () => {
    const page = bridge({
      readForm: async () => {
        throw new Error('no active tab');
      },
    });

    const res = await executeTool({ id: 'c5', tool: 'read_form' }, page);

    expect(res).toEqual({ id: 'c5', error: 'no active tab' });
  });
});

describe('mergeFrameOutcomes', () => {
  it('keeps the frame that actually filled a label over the frames that lack it', () => {
    const merged = mergeFrameOutcomes([
      [
        { label: 'Email', status: 'not_found' },
        { label: 'Phone', status: 'not_found' },
      ],
      [
        { label: 'Email', status: 'filled' },
        { label: 'Phone', status: 'not_found' },
      ],
    ]);

    expect(merged).toEqual([
      { label: 'Email', status: 'filled' },
      { label: 'Phone', status: 'not_found' },
    ]);
  });

  it('reports a combobox as deferred rather than as missing', () => {
    const merged = mergeFrameOutcomes([
      [{ label: 'Country', status: 'not_found' }],
      [{ label: 'Country', status: 'deferred_combobox' }],
    ]);

    expect(merged).toEqual([{ label: 'Country', status: 'deferred_combobox' }]);
  });

  it('returns nothing when no frame answered', () => {
    expect(mergeFrameOutcomes([])).toEqual([]);
  });
});

describe('parseToolCall', () => {
  it('accepts a well-formed call and defaults absent args', () => {
    expect(parseToolCall('{"id":"a","tool":"read_form"}')).toEqual({
      id: 'a',
      tool: 'read_form',
      args: {},
    });
  });

  it('drops frames it cannot correlate or dispatch', () => {
    expect(parseToolCall('not json')).toBeNull();
    expect(parseToolCall('{"tool":"read_form"}')).toBeNull();
    expect(parseToolCall('{"id":"a"}')).toBeNull();
    expect(parseToolCall(42)).toBeNull();
  });
});
