import { render, screen } from '@testing-library/svelte';
import { expect, it, vi } from 'vitest';
import EmployerOutboundClicks from './EmployerOutboundClicks.svelte';

const { getEmployerVacancyOutboundClicks } = vi.hoisted(() => ({ getEmployerVacancyOutboundClicks: vi.fn() }));

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('#lib/api.js', () => ({ api: { getEmployerVacancyOutboundClicks } }));

it('shows totals, days, countries and sources', async () => {
  getEmployerVacancyOutboundClicks.mockResolvedValue({
    days: [{ day: '2026-11-20', clicks: 12, visitors: 9 }],
    countries: [
      { key: 'DE', clicks: 10 },
      { key: '', clicks: 2 },
    ],
    sources: [{ key: 'web', clicks: 12 }],
    clicks: 12,
    visitors: 9,
  });
  render(EmployerOutboundClicks, { slug: 'acme-platform' });

  expect(await screen.findByText('2026-11-20')).toBeTruthy();
  expect(screen.getByText('Germany: 10')).toBeTruthy();
  expect(screen.getByText('Unknown: 2')).toBeTruthy();
  expect(screen.getByText('web: 12')).toBeTruthy();
  expect(getEmployerVacancyOutboundClicks).toHaveBeenCalledWith('acme-platform');
});

it('says so when nothing was clicked and when the load fails', async () => {
  getEmployerVacancyOutboundClicks.mockResolvedValueOnce({ days: [], countries: [], sources: [], clicks: 0, visitors: 0 });
  const { unmount } = render(EmployerOutboundClicks, { slug: 'a' });
  expect(await screen.findByText('No clicks to your site yet.')).toBeTruthy();
  unmount();

  getEmployerVacancyOutboundClicks.mockRejectedValueOnce(new Error('offline'));
  render(EmployerOutboundClicks, { slug: 'a' });
  expect(await screen.findByRole('alert')).toBeTruthy();
});
