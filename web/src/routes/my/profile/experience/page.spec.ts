import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ExperiencePage from './+page.svelte';

// A refused base-CV reset (e.g. the list-cap guard in internal/candidate/cvedit) already
// carries a specific, useful reason from the server — this page used to discard it and
// always show one generic string instead. See openspec change cv-refresh-error-message.

const { reseedBaseCv, askCvRefresh } = vi.hoisted(() => ({
  reseedBaseCv: vi.fn(),
  askCvRefresh: vi.fn(),
}));

const { StubApiError } = vi.hoisted(() => ({
  StubApiError: class StubApiError extends Error {
    constructor(
      public status: number,
      message: string,
    ) {
      super(message);
    }
  },
}));

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));
vi.mock('$lib/api', () => ({ api: { reseedBaseCv }, ApiError: StubApiError }));
vi.mock('$lib/cvRefreshDialog.svelte', () => ({ askCvRefresh }));
vi.mock('$lib/components/ExperienceBankView.svelte', async () => ({
  default: (await import('./ExperienceBankViewStub.svelte')).default,
}));

beforeEach(() => {
  reseedBaseCv.mockReset();
  askCvRefresh.mockReset().mockResolvedValue(true);
});

async function triggerBankMutated() {
  await fireEvent.click(screen.getByRole('button', { name: 'trigger bank mutated' }));
}

describe('experience page: base-CV refresh error', () => {
  it('shows the server-specific refusal reason', async () => {
    reseedBaseCv.mockRejectedValue(
      new StubApiError(
        409,
        'Staff Engineer at Contoso already has 20 bullets (the maximum). The edit was not applied and no existing bullets were deleted. Your existing bullets were kept.',
      ),
    );
    render(ExperiencePage);

    await triggerBankMutated();

    expect(
      await screen.findByText(/Staff Engineer at Contoso already has 20 bullets/),
    ).toBeTruthy();
  });

  it('falls back to a generic message when the failure carries no useful message', async () => {
    reseedBaseCv.mockRejectedValue('boom');
    render(ExperiencePage);

    await triggerBankMutated();

    expect(
      await screen.findByText(
        'Could not update your base CV. Try Reset from résumé in a tailoring workspace.',
      ),
    ).toBeTruthy();
  });
});
