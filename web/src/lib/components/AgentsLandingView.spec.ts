import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import AgentsLandingView from './AgentsLandingView.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));

describe('AgentsLandingView', () => {
  // The hub is for developers and their agents: the open tracks lead, Pro follows.
  it('opens on the MCP track and lists the open tracks before the Pro assistant', () => {
    render(AgentsLandingView);
    const tabs = screen.getAllByRole('tab').map((t) => t.textContent?.trim());
    expect(tabs).toEqual(['MCP', 'CLI', 'API', 'freehire Assistant · Pro']);
    expect(screen.getByRole('tab', { name: 'MCP' }).getAttribute('aria-selected')).toBe('true');
  });

  it('sends the API track to the Jobs API and the reference', () => {
    render(AgentsLandingView);
    expect(screen.getByRole('link', { name: 'Get an API key', hidden: true }).getAttribute('href')).toBe('/jobs-api');
    expect(screen.getByRole('link', { name: 'API reference', hidden: true }).getAttribute('href')).toBe('/docs/api');
  });
});
