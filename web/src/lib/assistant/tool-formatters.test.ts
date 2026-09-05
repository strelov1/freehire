import { describe, expect, it } from 'vitest';
import {
  callLine,
  CONFIRMATION_DECLINE_TEXT,
  groupTitle,
  isExpandable,
  nonEmptyInput,
  parseConfirmationRequest,
  previewToolInput,
  REQUEST_CONFIRMATION_TOOL,
  toolErrorMessage,
  toolLabel,
  type ToolCall,
} from './tool-formatters';

const call = (name: string, input: unknown = {}, extra: Partial<ToolCall> = {}): ToolCall => ({
  name,
  input,
  ...extra,
});

describe('toolLabel', () => {
  it('reads as intent, not as a function name', () => {
    expect(toolLabel(call('search_jobs'))).toBe('Searching jobs');
    expect(toolLabel(call('cv_edit'))).toBe('Updating your CV');
  });

  it('reads the tool name as a sentence for a tool the map does not know yet', () => {
    // A tool added on the backend must still render — and must not render as a raw
    // identifier, which is what put `experience_search` in front of users lowercased
    // and underscored while every labelled call beside it read as English.
    expect(toolLabel(call('brand_new_tool'))).toBe('Brand new tool');
  });
});

describe('groupTitle', () => {
  it('collapses repeated calls to their distinct intents', () => {
    const title = groupTitle([call('search_jobs'), call('search_jobs'), call('facets')]);
    expect(title).toBe('Searching jobs · Loading filters');
  });

  it('caps a long group with a counter', () => {
    const title = groupTitle([call('facets'), call('search_jobs'), call('get_job'), call('cv_get')]);
    expect(title).toBe('Loading filters · Searching jobs · +2');
  });

  it('is empty for no calls', () => {
    expect(groupTitle([])).toBe('');
  });
});

describe('callLine', () => {
  it('shows the query a search ran', () => {
    expect(callLine(call('search_jobs', { query: 'golang' }))).toBe('Searching jobs: golang');
  });

  it('summarises the filters when a search has no keyword', () => {
    const line = callLine(
      call('search_jobs', { filters: { seniority: ['senior'], regions: ['eu'] } }),
    );
    expect(line).toBe('Searching jobs: seniority=senior, regions=eu');
  });

  it('shows the slug a vacancy call addresses', () => {
    expect(callLine(call('apply_job', { slug: 'go-dev-acme' }))).toBe(
      'Marking as applied: go-dev-acme',
    );
  });

  // cv_edit takes a batch, so the useful detail is how much it changed at once — there are no
  // named ops any more to report.
  it('shows how many edits a CV change carried', () => {
    expect(callLine(call('cv_edit', { ops: [{ kind: 'set', path: 'summary' }] }))).toBe(
      'Updating your CV: 1 edit',
    );
    expect(
      callLine(call('cv_edit', { ops: [{ kind: 'set', path: 'summary' }, { kind: 'remove', path: 'skills[0]' }] })),
    ).toBe('Updating your CV: 2 edits');
  });

  it('shows the label alone when there is nothing identifying to add', () => {
    expect(callLine(call('facets'))).toBe('Loading filters');
  });
});

describe('toolErrorMessage', () => {
  it('unwraps the error envelope the backend sends the model', () => {
    const failed = call(
      'search_jobs',
      {},
      { isError: true, result: '{"error":"search is not available"}' },
    );
    expect(toolErrorMessage(failed)).toBe('search is not available');
  });

  it('is null for a call that succeeded', () => {
    expect(toolErrorMessage(call('facets', {}, { result: '{"total":5}' }))).toBeNull();
  });

  it('falls back to the raw payload when it is not an envelope', () => {
    const failed = call('facets', {}, { isError: true, result: 'boom' });
    expect(toolErrorMessage(failed)).toBe('boom');
  });
});

describe('isExpandable', () => {
  it('is flat for a single argument-less call', () => {
    expect(isExpandable([call('facets')])).toBe(false);
  });

  it('expands a call that carries arguments', () => {
    expect(isExpandable([call('search_jobs', { query: 'go' })])).toBe(true);
  });

  it('expands a failed call even with no arguments, so the reason is reachable', () => {
    expect(isExpandable([call('facets', {}, { isError: true, result: '{"error":"down"}' })])).toBe(
      true,
    );
  });

  it('expands any group of more than one call', () => {
    expect(isExpandable([call('facets'), call('facets')])).toBe(true);
  });
});

describe('input helpers', () => {
  it('treats an empty object as no input', () => {
    expect(nonEmptyInput({})).toBe(false);
    expect(nonEmptyInput({ a: 1 })).toBe(true);
    expect(nonEmptyInput(null)).toBe(false);
  });

  it('previews input as truncated JSON', () => {
    expect(previewToolInput({ query: 'go' })).toBe('{"query":"go"}');
  });
});

describe('parseConfirmationRequest', () => {
  it('reads the claim and question off a request_confirmation call', () => {
    const parsed = parseConfirmationRequest(
      call(REQUEST_CONFIRMATION_TOOL, { claim: 'Built Reelmente.app with React', question: 'Is that right?' }),
    );
    expect(parsed).toEqual({ claim: 'Built Reelmente.app with React', question: 'Is that right?' });
  });

  it('defaults a missing question to an empty string', () => {
    const parsed = parseConfirmationRequest(call(REQUEST_CONFIRMATION_TOOL, { claim: 'Built it' }));
    expect(parsed).toEqual({ claim: 'Built it', question: '' });
  });

  it('is null for any other tool', () => {
    expect(parseConfirmationRequest(call('cv_edit', { claim: 'Built it' }))).toBeNull();
  });

  it('is null when the claim is missing or blank', () => {
    expect(parseConfirmationRequest(call(REQUEST_CONFIRMATION_TOOL, {}))).toBeNull();
    expect(parseConfirmationRequest(call(REQUEST_CONFIRMATION_TOOL, { claim: '  ' }))).toBeNull();
    expect(parseConfirmationRequest(call(REQUEST_CONFIRMATION_TOOL, null))).toBeNull();
  });

  it('exposes a fixed decline message so the button and its test never drift apart', () => {
    expect(CONFIRMATION_DECLINE_TEXT.length).toBeGreaterThan(0);
  });
});
