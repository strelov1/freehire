import { describe, it, expect } from 'vitest';
import { viewAtsDelta } from './atsdelta';
import type { CvAtsDelta } from '$lib/types';

function resp(over: Partial<CvAtsDelta> = {}): CvAtsDelta {
  return {
    available: true,
    base_cv_id: 'base-1',
    delta: {
      base: 60,
      tailored: 69,
      change: 9,
      regressed: false,
      categories: [
        { id: 'keyword_strength', label: 'Keyword Strength', base: 10, tailored: 18, change: 8 },
        { id: 'format_compliance', label: 'Format Compliance', base: 20, tailored: 21, change: 1 },
      ],
    },
    ...over,
  };
}

describe('viewAtsDelta', () => {
  it('is absent when there is nothing to show', () => {
    expect(viewAtsDelta(null)).toBeNull();
    expect(viewAtsDelta({ available: false, reason: 'CV rendering is not available' })).toBeNull();
    expect(viewAtsDelta({ available: true })).toBeNull();
  });

  it('signs the overall change', () => {
    expect(viewAtsDelta(resp())?.overall.text).toBe('+9');
  });

  it('renders a drop with a real minus sign, not a hyphen', () => {
    const v = viewAtsDelta(
      resp({
        delta: {
          base: 70,
          tailored: 55,
          change: -15,
          regressed: true,
          categories: [
            { id: 'format_compliance', label: 'Format Compliance', base: 30, tailored: 15, change: -15 },
          ],
        },
      }),
    );
    expect(v?.overall.text).toBe('−15');
  });

  it('says nothing changed rather than showing a bare zero', () => {
    const v = viewAtsDelta(
      resp({ delta: { base: 60, tailored: 60, change: 0, regressed: false, categories: [] } }),
    );
    expect(v?.overall.text).toBe('no change');
  });

  it('warns on a regression and names the category that fell furthest', () => {
    const v = viewAtsDelta(
      resp({
        delta: {
          base: 70,
          tailored: 58,
          change: -12,
          regressed: true,
          worst_category: 'format_compliance',
          categories: [
            { id: 'keyword_strength', label: 'Keyword Strength', base: 20, tailored: 22, change: 2 },
            { id: 'format_compliance', label: 'Format Compliance', base: 30, tailored: 16, change: -14 },
          ],
        },
      }),
    );
    expect(v?.regressed).toBe(true);
    expect(v?.warning).toContain('Format Compliance');
    expect(v?.warning).toContain('12');
  });

  it('warns without naming a category when the id is not among the rows', () => {
    const v = viewAtsDelta(
      resp({
        delta: {
          base: 70,
          tailored: 65,
          change: -5,
          regressed: true,
          worst_category: 'a_category_the_client_does_not_know',
          categories: [
            { id: 'keyword_strength', label: 'Keyword Strength', base: 20, tailored: 15, change: -5 },
          ],
        },
      }),
    );
    expect(v?.regressed).toBe(true);
    expect(v?.warning).toBeTruthy();
    expect(v?.warning).not.toContain('undefined');
  });

  it('does not warn when the score held or rose', () => {
    expect(viewAtsDelta(resp())?.warning).toBeUndefined();
    expect(viewAtsDelta(resp())?.regressed).toBe(false);
  });

  it('carries every category through in the order served, each signed', () => {
    const rows = viewAtsDelta(resp())?.rows ?? [];
    expect(rows.map((r) => r.id)).toEqual(['keyword_strength', 'format_compliance']);
    expect(rows.map((r) => r.text)).toEqual(['+8', '+1']);
  });
});
