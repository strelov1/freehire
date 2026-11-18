import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import CoverLetter from './CoverLetter.svelte';
vi.mock('@lucide/svelte', () => ({ Copy() {}, Check() {}, FileText() {}, Loader2() {} }));
const { get, stream, getEvidence, saveEvidence } = vi.hoisted(() => ({
  get: vi.fn(),
  stream: vi.fn(),
  getEvidence: vi.fn(),
  saveEvidence: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({
  api: {
    getCoverLetter: get,
    openCoverLetterStream: stream,
    getCoverLetterEvidence: getEvidence,
    saveCoverLetterEvidence: saveEvidence,
  },
  trackPlanRefusal: vi.fn(),
}));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
beforeEach(() => {
  get.mockReset().mockResolvedValue({ present: false });
  stream.mockReset();
  saveEvidence.mockReset();
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

it('keeps evidence open and direct writing blocked until a save finishes', async () => {
  let finish: (value: { revision: number; atom_ids: string[] }) => void = () => {};
  saveEvidence.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  getEvidence.mockResolvedValue({
    revision: 0,
    explicit: false,
    atom_ids: [],
    suggested_ids: ['one'],
    invalid_ids: [],
    available: [{ id: 'one', claim: 'Built a worker', provenance: 'manual' }],
  });
  render(CoverLetter, { cvId: 'cv' });
  await fireEvent.click(await screen.findByRole('button', { name: 'Review evidence' }));
  await screen.findByRole('checkbox', { name: 'Built a worker' });
  await fireEvent.click(screen.getByRole('button', { name: 'Save selection' }));
  await waitFor(() => expect(saveEvidence).toHaveBeenCalledOnce());
  expect(screen.getByRole('button', { name: 'Close evidence' })).toHaveProperty('disabled', true);
  await fireEvent.click(screen.getByRole('button', { name: 'Close evidence' }));
  expect(screen.getByRole('button', { name: 'Write the letter' })).toHaveProperty('disabled', true);
  expect(stream).not.toHaveBeenCalled();
  finish({ revision: 1, atom_ids: ['one'] });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Close evidence' })).toHaveProperty('disabled', false));
  await fireEvent.click(screen.getByRole('button', { name: 'Close evidence' }));
  expect(screen.getByRole('button', { name: 'Write the letter' })).toHaveProperty('disabled', false);
});
