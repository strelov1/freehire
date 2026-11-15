import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import CoverLetterEvidence from './CoverLetterEvidence.svelte';

const { get, save } = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getCoverLetterEvidence: get, saveCoverLetterEvidence: save } }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
const evidence = () => ({
  revision: 0,
  explicit: false,
  atom_ids: [],
  suggested_ids: ['one'],
  invalid_ids: [],
  available: [
    { id: 'one', claim: 'Built a Go worker', provenance: 'manual' },
    { id: 'two', claim: 'Reduced database latency', provenance: 'cv_import' },
  ],
});
beforeEach(() => {
  get.mockReset().mockResolvedValue(evidence());
  save.mockReset().mockResolvedValue({ revision: 1, atom_ids: ['one'] });
});
afterEach(cleanup);

it('preselects suggested facts and saves an explicit empty choice without drafting', async () => {
  const onwrite = vi.fn();
  render(CoverLetterEvidence, { cvId: 'cv', onwrite });
  const fact = await screen.findByRole('checkbox', { name: 'Built a Go worker' });
  expect(fact).toHaveProperty('checked', true);
  await fireEvent.click(fact);
  expect(screen.getByRole('button', { name: 'Write using these facts' })).toHaveProperty('disabled', true);
  await fireEvent.click(screen.getByRole('button', { name: 'Save selection' }));
  await waitFor(() => expect(save).toHaveBeenCalledWith('cv', 0, []));
  expect(onwrite).not.toHaveBeenCalled();
});

it('keeps local choices after a failed save and never starts generation', async () => {
  save.mockRejectedValue(new Error('Save failed'));
  const onwrite = vi.fn();
  render(CoverLetterEvidence, { cvId: 'cv', onwrite });
  await screen.findByRole('checkbox', { name: 'Built a Go worker' });
  await fireEvent.click(screen.getByRole('button', { name: 'Write using these facts' }));
  await screen.findByText('Save failed');
  expect(screen.getByRole('checkbox', { name: 'Built a Go worker' })).toHaveProperty('checked', true);
  expect(onwrite).not.toHaveBeenCalled();
});

it('saves selected facts before writing', async () => {
  const onwrite = vi.fn();
  render(CoverLetterEvidence, { cvId: 'cv', onwrite });
  await screen.findByRole('checkbox', { name: 'Built a Go worker' });
  await fireEvent.click(screen.getByRole('button', { name: 'Write using these facts' }));
  await waitFor(() => expect(onwrite).toHaveBeenCalledOnce());
  expect(save).toHaveBeenCalledWith('cv', 0, ['one']);
});

it('retains choices after a conflict and retries against the refreshed revision', async () => {
  const conflict = Object.assign(new Error('Conflict'), { status: 409 });
  save.mockRejectedValueOnce(conflict).mockResolvedValueOnce({ revision: 3, atom_ids: ['one'] });
  get
    .mockResolvedValueOnce(evidence())
    .mockResolvedValueOnce({ ...evidence(), revision: 2, explicit: true, atom_ids: ['two'] });
  render(CoverLetterEvidence, { cvId: 'cv', onwrite: vi.fn() });
  await screen.findByRole('checkbox', { name: 'Built a Go worker' });
  await fireEvent.click(screen.getByRole('button', { name: 'Save selection' }));
  await screen.findByText(/saved selection changed in another tab/);
  expect(screen.getByRole('checkbox', { name: 'Built a Go worker' })).toHaveProperty('checked', true);
  await fireEvent.click(screen.getByRole('button', { name: 'Save selection' }));
  await waitFor(() => expect(save).toHaveBeenLastCalledWith('cv', 2, ['one']));
});

it('ignores evidence arriving after navigation to another vacancy', async () => {
  let finish: (value: ReturnType<typeof evidence>) => void = () => {};
  get.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { rerender } = render(CoverLetterEvidence, { cvId: 'first', onwrite: vi.fn() });
  await waitFor(() => expect(get).toHaveBeenCalledWith('first'));
  await rerender({ cvId: 'second', onwrite: vi.fn() });
  await screen.findByRole('checkbox', { name: 'Built a Go worker' });
  finish({ ...evidence(), available: [{ id: 'old', claim: 'Stale vacancy fact', provenance: 'manual' }] });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(screen.queryByText('Stale vacancy fact')).toBeNull();
});

it('shows the employment behind an achievement', async () => {
  get.mockResolvedValue({
    ...evidence(),
    employment_labels: { role: 'Acme · Engineer' },
    available: [{ id: 'one', claim: 'Built a Go worker', provenance: 'manual', employment_id: 'role' }],
  });
  render(CoverLetterEvidence, { cvId: 'cv', onwrite: vi.fn() });
  await screen.findByText(/Acme · Engineer/);
});
