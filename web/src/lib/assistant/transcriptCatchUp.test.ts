import { afterEach, expect, it, vi } from 'vitest';
import { TranscriptCatchUp } from './transcriptCatchUp';

afterEach(() => vi.unstubAllGlobals());

it('clears feedback when the refreshed rehearsal has a newer answer', async () => {
  const report = {
    strength: 'Specific metric',
    improvement: 'Add the result',
    quote: 'p95',
    question: 'How did it go?',
  };
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ data: { messages: [], interview_report: report } }))
    .mockResolvedValueOnce(
      Response.json({ data: { messages: [{ seq: 1, role: 'user', content: { text: 'p95 fell' } }] } }),
    );
  vi.stubGlobal('fetch', fetch);
  const show = vi.fn();
  const transcript = new TranscriptCatchUp({
    isSupport: false,
    activeId: () => 'rehearsal',
    streaming: () => false,
    show,
  });

  await transcript.reload('rehearsal');
  expect(show).toHaveBeenLastCalledWith(expect.anything(), report);
  await transcript.reload('rehearsal');
  expect(show).toHaveBeenLastCalledWith(
    expect.objectContaining({ messages: [expect.objectContaining({ text: 'p95 fell' })] }),
    undefined,
  );
});

it('ignores a refresh that started before the report finished', async () => {
  const { promise, resolve } = Promise.withResolvers<Response>();
  vi.stubGlobal('fetch', vi.fn().mockReturnValue(promise));
  const show = vi.fn();
  const transcript = new TranscriptCatchUp({
    isSupport: false,
    activeId: () => 'rehearsal',
    streaming: () => false,
    show,
  });

  const reload = transcript.reload('rehearsal');
  transcript.invalidate();
  resolve(Response.json({ data: { messages: [] } }));
  await reload;
  expect(show).not.toHaveBeenCalled();
});
