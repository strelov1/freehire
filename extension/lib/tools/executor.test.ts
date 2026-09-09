import { describe, it, expect } from 'vitest';
// The raw text, not the parsed object: parseToolCall takes what comes off the socket,
// and parsing it here would skip the half of the boundary that reads the wire.
import fillSimpleFrame from './testdata/fill-simple-call.json?raw';
import { executeTool, mergeComboboxReplies, mergeFrameOutcomes, type PageBridge } from './executor';
import { parseToolCall } from './wire';
import type { FramedField } from './wire';
import type { ComboboxStep, LabelFill } from '../protocol';

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

  // The scope is what stops a write landing in the wrong one of two forms carrying
  // the same label. A harness reads it off `read_form` and sends it back here; this
  // reader dropped it, so every agent-planned fill arrived unscoped and `form.ts`
  // broadcast it to every frame and took the first match.
  it('carries the frame and form a fill names through to the page', async () => {
    let seen: LabelFill[] = [];
    const page = bridge({
      fillSimple: async (fills) => {
        seen = fills;
        return fills.map((f) => ({ label: f.label, status: 'filled' as const }));
      },
    });

    await executeTool(
      {
        id: 'c5',
        tool: 'fill_simple',
        args: { fills: [{ label: 'Email', value: 'a@b.c', frame: 1, form: 2 }] },
      },
      page,
    );

    expect(seen).toEqual([{ label: 'Email', value: 'a@b.c', frame: 1, form: 2 }]);
  });

  // -1 is not a malformed form index, it is the documented one for a question
  // standing outside any <form> — which is how Ashby renders its application (see
  // FormField.form, and formIndex's own return). Reading it as "unscoped" leaves
  // every Ashby fill unaddressed, and on a page that also carries a job-alert form
  // asking the same question, unaddressed now means refused: nothing is written.
  it('keeps form -1, the index of a question outside any form', async () => {
    let seen: LabelFill[] = [];
    const page = bridge({
      fillSimple: async (fills) => {
        seen = fills;
        return fills.map((f) => ({ label: f.label, status: 'filled' as const }));
      },
    });

    await executeTool(
      {
        id: 'c11',
        tool: 'fill_simple',
        args: { fills: [{ label: 'Email', value: 'a@b.c', frame: 0, form: -1 }] },
      },
      page,
    );

    expect(seen).toEqual([{ label: 'Email', value: 'a@b.c', frame: 0, form: -1 }]);
  });

  // A frame index has no such case: frames are counted from the top document at 0,
  // so a negative one is malformed however it arrived.
  it('rejects a negative frame, which has no meaning', async () => {
    let seen: LabelFill[] = [];
    const page = bridge({
      fillSimple: async (fills) => {
        seen = fills;
        return fills.map((f) => ({ label: f.label, status: 'filled' as const }));
      },
    });

    await executeTool(
      { id: 'c12', tool: 'fill_simple', args: { fills: [{ label: 'Email', value: 'a@b.c', frame: -1 }] } },
      page,
    );

    expect(seen).toEqual([{ label: 'Email', value: 'a@b.c', frame: undefined, form: undefined }]);
  });

  // A scope is optional — an unscoped fill is still offered to every frame — but a
  // malformed one must not be read as frame 0, which is the top document and a real
  // target. Absent and unreadable both mean "not scoped".
  it('leaves an absent or unreadable scope unscoped rather than defaulting it', async () => {
    let seen: LabelFill[] = [];
    const page = bridge({
      fillSimple: async (fills) => {
        seen = fills;
        return fills.map((f) => ({ label: f.label, status: 'filled' as const }));
      },
    });

    await executeTool(
      {
        id: 'c6',
        tool: 'fill_simple',
        args: {
          fills: [
            { label: 'Email', value: 'a@b.c' },
            { label: 'Phone', value: '123', frame: 'top', form: 1.5 },
          ],
        },
      },
      page,
    );

    expect(seen).toEqual([
      { label: 'Email', value: 'a@b.c', frame: undefined, form: undefined },
      { label: 'Phone', value: '123', frame: undefined, form: undefined },
    ]);
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

// The check that spans the two ends of the wire.
//
// hire's autofill agent has tagged every fill with its frame since frame scoping was
// introduced, and a Go test asserts that it does — while `readFills` here destructured
// only {label, value}, so the tag was dropped on arrival and `form.ts` broadcast the
// fill to every frame and matched it by label alone. Both sides' tests were green: the
// Go one checks the sender, the ones above check this reader against arguments written
// by hand here. Neither can see a field one side sends and the other ignores.
//
// The fixture is written from hire's live `autofillagent.Fill` struct
// (internal/ai/autofillagent/wirefixture_test.go, which fails if it drifts), so this
// reads the real frame rather than a second hand-written copy of it that would drift
// the same way.
describe('the fill_simple frame hire actually sends', () => {
  it('reaches the page with both of its scopes intact', async () => {
    let seen: LabelFill[] = [];
    const page = bridge({
      fillSimple: async (fills) => {
        seen = fills;
        return fills.map((f) => ({ label: f.label, status: 'filled' as const }));
      },
    });

    const call = parseToolCall(fillSimpleFrame);
    // Thrown rather than asserted: a fixture that no longer parses is a broken
    // fixture, and the assertion below would report it as a scope that went missing.
    if (!call) throw new Error('the committed fill_simple fixture is not a well-formed tool call');

    const res = await executeTool(call, page);

    expect(res.error).toBeUndefined();
    expect(seen).toEqual([{ label: 'Email', value: 'ilya@example.com', frame: 1, form: 2 }]);
  });

  // Matching the values is only half of it. The defect was a field hire SENT that
  // this side never read, and an assertion over what `readFills` produced cannot see
  // one: an ignored key simply does not appear in the result. So the fixture's own
  // keys are checked against the set this reader handles — add a field to
  // `autofillagent.Fill`, regenerate, and this fails until `readFills` is taught it.
  it('has no key this reader ignores', () => {
    const READ_BY_READFILLS = new Set(['label', 'value', 'frame', 'form']);

    const { args } = JSON.parse(fillSimpleFrame) as { args: { fills: Record<string, unknown>[] } };
    const ignored = args.fills.flatMap((fill) => Object.keys(fill).filter((k) => !READ_BY_READFILLS.has(k)));

    expect(ignored, `readFills ignores ${ignored.join(', ')} — teach it, or drop the field`).toEqual([]);
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

  // A refusal is an answer: the frame that reported it is the one holding the
  // question, and the frames answering `not_found` are the ones that never saw it.
  // Folding the negatives over it would tell the harness the page does not ask
  // something it does ask — and hide the one status it could act on.
  it('keeps a refusal over the frames that never held the control', () => {
    const merged = mergeFrameOutcomes([
      [{ label: 'Email', status: 'not_found' }],
      [{ label: 'Email', status: 'ambiguous' }],
      [{ label: 'Email', status: 'not_found' }],
    ]);

    expect(merged).toEqual([{ label: 'Email', status: 'ambiguous' }]);
  });

  // The negative comes FIRST on purpose: ties keep the frame that answered first,
  // so an informative status placed first would survive a flattened rank table and
  // the test would pass without testing the ranking at all.
  it('keeps wrong_form over a frame that does not carry the label at all', () => {
    const merged = mergeFrameOutcomes([
      [{ label: 'Email', status: 'not_found' }],
      [{ label: 'Email', status: 'wrong_form' }],
    ]);

    expect(merged).toEqual([{ label: 'Email', status: 'wrong_form' }]);
  });

  it('keeps not_fillable over a frame that does not carry the label at all', () => {
    const merged = mergeFrameOutcomes([
      [{ label: 'Email', status: 'not_found' }],
      [{ label: 'Email', status: 'not_fillable' }],
    ]);

    expect(merged).toEqual([{ label: 'Email', status: 'not_fillable' }]);
  });

  // A write that landed is what actually happened, and no other frame's refusal
  // undoes it — so `filled` stays the top of the order.
  it('keeps a write that landed over another frame refusing the same label', () => {
    const merged = mergeFrameOutcomes([
      [{ label: 'Email', status: 'ambiguous' }],
      [{ label: 'Email', status: 'filled' }],
    ]);

    expect(merged).toEqual([{ label: 'Email', status: 'filled' }]);
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
