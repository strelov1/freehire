import { describe, expect, it } from 'vitest';
import {
  callLine,
  groupTitle,
  isExpandable,
  nonEmptyInput,
  previewToolInput,
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
  // The panel's own tool. Without a label the transcript shows the raw function
  // name for the one call that only ever happens here.
  it('labels reading the current page', () => {
    expect(toolLabel(call('read_current_page'))).toBe('Reading the page you are on');
  });

  it('reads as intent, not as a function name', () => {
    expect(toolLabel(call('search_jobs'))).toBe('Searching jobs');
    expect(toolLabel(call('cv_edit'))).toBe('Updating your CV');
  });

  it('falls back to the tool name for a tool the map does not know yet', () => {
    // A tool added on the backend must still render rather than showing blank.
    expect(toolLabel(call('brand_new_tool'))).toBe('brand_new_tool');
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
