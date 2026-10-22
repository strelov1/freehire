import { afterEach, describe, expect, it, vi } from 'vitest';

const { goto, match } = vi.hoisted(() => ({ goto: vi.fn(), match: vi.fn() }));
vi.mock('$app/navigation', () => ({ goto }));
vi.mock('$app/paths', () => ({ match }));

import { gotoPath } from './gotoPath';

describe('gotoPath', () => {
  const assign = vi.fn();

  afterEach(() => {
    goto.mockReset();
    match.mockReset();
    assign.mockReset();
    vi.unstubAllGlobals();
  });

  it('navigates client-side to a page of this app', async () => {
    match.mockResolvedValue({ id: '/jobs', params: {} });
    vi.stubGlobal('location', { assign });

    await gotoPath('/jobs?remote=true', { replaceState: true });

    expect(goto).toHaveBeenCalledWith('/jobs?remote=true', { replaceState: true });
    expect(assign).not.toHaveBeenCalled();
  });

  // The CLI/MCP sign-in returns to the Go API's authorize endpoint. SvelteKit 3's goto()
  // rejects a URL no route matches, so handing it this path strands the visitor on /signin.
  it('loads a path no route matches as a full page', async () => {
    match.mockResolvedValue(null);
    vi.stubGlobal('location', { assign });

    await gotoPath('/api/v1/oauth/authorize?client_id=cli&state=x');

    expect(goto).not.toHaveBeenCalled();
    expect(assign).toHaveBeenCalledWith('/api/v1/oauth/authorize?client_id=cli&state=x');
  });
});
