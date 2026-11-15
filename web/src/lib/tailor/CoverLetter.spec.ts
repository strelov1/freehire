import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import CoverLetter from './CoverLetter.svelte';
vi.mock('@lucide/svelte', () => ({ Copy() {}, Check() {}, FileText() {}, Loader2() {} }));
const { get, stream, getEvidence } = vi.hoisted(() => ({ get: vi.fn(), stream: vi.fn(), getEvidence: vi.fn() }));
vi.mock('#lib/api.js', () => ({
  api: { getCoverLetter: get, openCoverLetterStream: stream, getCoverLetterEvidence: getEvidence },
  trackPlanRefusal: vi.fn(),
}));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
beforeEach(() => {
  get.mockReset().mockResolvedValue({ present: false });
  stream.mockReset();
  getEvidence.mockReset().mockResolvedValue({
    revision: 0,
    explicit: false,
    atom_ids: [],
    suggested_ids: [],
    invalid_ids: [],
    available: [],
  });
});
afterEach(cleanup);
it('retains an error sent by the generation stream', async () => {
  stream.mockResolvedValue(
    new Response('event: stream_error\ndata: {"error":"Model unavailable"}\n\n', { status: 200 }),
  );
  render(CoverLetter, { cvId: 'cv' });
  await fireEvent.click(await screen.findByRole('button', { name: 'Write the letter' }));
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Write the letter' })).toHaveProperty('disabled', false),
  );
  await screen.findByText('Model unavailable');
  expect(get).toHaveBeenCalledTimes(1);
});
it('disables the direct write action while reviewing evidence', async () => {
  render(CoverLetter, { cvId: 'cv' });
  await fireEvent.click(await screen.findByRole('button', { name: 'Review evidence' }));
  expect(screen.getByRole('button', { name: 'Write the letter' })).toHaveProperty('disabled', true);
});
it('disables direct generation for a saved empty selection', async () => {
  get.mockResolvedValue({ present: false, evidence_blocked: true });
  render(CoverLetter, { cvId: 'cv' });
  expect(await screen.findByRole('button', { name: 'Write the letter' })).toHaveProperty('disabled', true);
});
