import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, the choice jobRowApplyButton.test.ts makes and for the same reason:
// `web/` has no component-test infrastructure, so a mounted test cannot exist here. It pins
// the spec's "an applied candidate sees it" (openspec/changes/add-in-app-apply): the job
// page's on-site Apply reads whether the APPLICATION was sent, never the tracker's applied
// mark — a vacancy marked applied by hand must still take the application, and a delivered
// one must not offer Apply again because tracking lagged.
const VIEW = readFileSync(join(import.meta.dirname, 'JobView.svelte'), 'utf8');

const start = VIEW.indexOf('{#if job.direct_apply}', VIEW.indexOf('{#snippet applyCta'));
const directBranch = start === -1 ? '' : VIEW.slice(start, VIEW.indexOf('{:else}', start));

describe('JobView on-site Apply', () => {
  it('finds the branch it audits', () => {
    expect(directBranch).not.toBe('');
  });

  it("reads the application's own sent state, not tracking", () => {
    expect(directBranch).toContain('disabled={directApplied');
    expect(directBranch).not.toContain('disabled={applied');
    expect(VIEW).toMatch(/directApplied = f\.applied/);
  });

  it('opens the form, never an outside link', () => {
    expect(directBranch).toContain('onclick={onDirectApplyClick}');
    expect(directBranch).not.toContain('href=');
  });
});

// The dialog sends the CV tailored for THIS vacancy or the uploaded CV — never a pick from
// every CV the candidate owns, which once led with one tailored for another vacancy.
describe('DirectApplyDialog CV choice', () => {
  const DIALOG = readFileSync(join(import.meta.dirname, 'DirectApplyDialog.svelte'), 'utf8');

  it('does not list every CV', () => {
    expect(DIALOG).not.toContain('listCvs');
  });

  it('attaches the tailored CV by itself when there is one', () => {
    expect(DIALOG).toContain("cv = f.cvs.tailored ? 'tailored' : null;");
  });

  it('sends only the tailored CV or the uploaded one', () => {
    expect(DIALOG).toContain('{ cv_id: form.cvs.tailored.id }');
    expect(DIALOG).toContain('{ resume: true as const }');
  });

  it('offers both ways in when there is neither', () => {
    expect(DIALOG).toContain('Upload my CV');
    expect(DIALOG).toContain('onclick={onTailor}');
  });
});
