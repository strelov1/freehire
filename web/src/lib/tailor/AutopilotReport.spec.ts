import { render, screen } from '@testing-library/svelte';
import { expect, it } from 'vitest';
import AutopilotReport from './AutopilotReport.svelte';

it('shows partial evidence and the remaining gap without counting it as closed', () => {
  render(AutopilotReport, {
    report: [
      { requirement: 'Azure AI Search', status: 'partial', unverified: 'The named tool was not confirmed.' },
      { requirement: 'RAG', status: 'closed_bank' },
    ],
    onRerun: () => {},
  });
  expect(screen.getByRole('heading', { name: 'Autopilot · 1 of 2 closed' })).toBeTruthy();
  expect(screen.getByText('Partially supported')).toBeTruthy();
  expect(screen.getByText('Still unverified: The named tool was not confirmed.')).toBeTruthy();
});
