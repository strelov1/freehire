import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import JobsApiPlayground from './JobsApiPlayground.svelte';

afterEach(() => vi.unstubAllGlobals());

describe('JobsApiPlayground', () => {
  it('shows a sample until run, and the code for the current inputs in each language', async () => {
    render(JobsApiPlayground);
    expect(screen.getByText(/Sample response/)).toBeTruthy();
    expect(
      screen.getByText(/curl "https:\/\/freehire\.me\/api\/v1\/jobs\/search\?q=golang&countries=de&limit=3"/),
    ).toBeTruthy();

    await fireEvent.click(screen.getByRole('tab', { name: 'Python' }));
    expect(screen.getByText(/import requests/)).toBeTruthy();
  });

  it('runs a keyless request against the live endpoint and prints the trimmed answer', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: [
            {
              title: 'Go Engineer',
              company: 'Acme',
              source: 'greenhouse',
              url: 'https://x',
              skills: ['go', 'grpc'],
              description: 'long body',
            },
          ],
          meta: { total: 42, limit: 3, offset: 0 },
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal('fetch', fetchMock);
    render(JobsApiPlayground);
    await fireEvent.click(screen.getByRole('button', { name: /Run request/ }));

    await waitFor(() => expect(screen.getByText(/42 matches/)).toBeTruthy());
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/jobs/search?q=golang&countries=de&limit=3', expect.anything());
    expect(screen.getByText(/"Go Engineer"/)).toBeTruthy();
    expect(screen.queryByText(/long body/)).toBeNull();
    expect(screen.getByText(/"skills": \["go", "grpc"\]/)).toBeTruthy();
  });

  it('says so when the keyless budget is spent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 429 })));
    render(JobsApiPlayground);
    await fireEvent.click(screen.getByRole('button', { name: /Run request/ }));
    await waitFor(() => expect(screen.getByText(/keyless budget/)).toBeTruthy());
  });

  it('says when nothing matches instead of printing an empty list as a success', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ data: [], meta: { total: 0, limit: 3, offset: 0 } }), { status: 200 }),
        ),
    );
    render(JobsApiPlayground);
    await fireEvent.click(screen.getByRole('button', { name: /Run request/ }));
    await waitFor(() => expect(screen.getByText(/no open postings match/)).toBeTruthy());
  });
});
