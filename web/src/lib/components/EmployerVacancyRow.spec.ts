import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, it, vi } from 'vitest';
import type { Job } from '#lib/types.js';
import EmployerVacancyRow from './EmployerVacancyRow.svelte';

const { getEmployerVacancyAnalytics } = vi.hoisted(() => ({ getEmployerVacancyAnalytics: vi.fn() }));

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('#lib/api.js', () => ({
  api: {
    getEmployerVacancyAnalytics,
    getEmployerVacancyOutboundClicks: vi
      .fn()
      .mockResolvedValue({ days: [], countries: [], sources: [], clicks: 0, visitors: 0 }),
  },
}));

it('loads a closed vacancy’s analytics on demand for a recruiter', async () => {
  getEmployerVacancyAnalytics.mockResolvedValue({ views: 40, apply_clicks: 10, applications: 3 });
  render(EmployerVacancyRow, {
    job: {
      public_slug: 'acme-platform',
      title: 'Platform Engineer',
      source: 'employer',
      closed_at: '2026-11-21',
    } as Job,
    canManage: false,
    closing: false,
    reopening: false,
    reopenError: null,
    onEdit: vi.fn(),
    onClose: vi.fn(),
    onReopen: vi.fn(),
    onDelete: vi.fn(),
  });

  expect(getEmployerVacancyAnalytics).not.toHaveBeenCalled();
  await fireEvent.click(screen.getByRole('button', { name: 'Analytics' }));
  const report = within(await screen.findByRole('region', { name: 'Analytics' }));
  for (const value of ['40', '10', '3', '25%']) {
    expect(await report.findByText(value)).toBeTruthy();
  }
  expect(getEmployerVacancyAnalytics).toHaveBeenCalledWith('acme-platform');
  expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull();
});

it('can reopen a failed report and leaves the click rate undefined without views', async () => {
  getEmployerVacancyAnalytics
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValueOnce({ views: 0, apply_clicks: 0, applications: 2 });
  render(EmployerVacancyRow, {
    job: { public_slug: 'acme-platform', title: 'Platform Engineer', source: 'employer' } as Job,
    closing: false,
    reopening: false,
    reopenError: null,
    onEdit: vi.fn(),
    onClose: vi.fn(),
    onReopen: vi.fn(),
    onDelete: vi.fn(),
  });

  const analytics = screen.getByRole('button', { name: 'Analytics' });
  await fireEvent.click(analytics);
  expect(await screen.findByRole('alert')).toBeTruthy();
  await fireEvent.click(analytics);
  expect(screen.queryByRole('region', { name: 'Analytics' })).toBeNull();
  await fireEvent.click(analytics);
  expect(await screen.findByText('—')).toBeTruthy();
  expect(screen.getByText('2')).toBeTruthy();
});

it('lets the team delete a vacancy it published', async () => {
  const onDelete = vi.fn();
  render(EmployerVacancyRow, {
    job: { public_slug: 'acme-platform', title: 'Platform Engineer', source: 'employer' } as Job,
    closing: false,
    reopening: false,
    reopenError: null,
    onEdit: vi.fn(),
    onClose: vi.fn(),
    onReopen: vi.fn(),
    onDelete,
  });

  await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onDelete).toHaveBeenCalledOnce();
  expect(screen.queryByText(/Collected from/)).toBeNull();
});

it('shows a collected vacancy read-only, with its source and analytics', async () => {
  getEmployerVacancyAnalytics.mockResolvedValue({ views: 7, apply_clicks: 0, applications: 0 });
  render(EmployerVacancyRow, {
    job: { public_slug: 'acme-ats', title: 'SRE', source: 'greenhouse', url: 'https://boards.example/1' } as Job,
    closing: false,
    reopening: false,
    reopenError: null,
    onEdit: vi.fn(),
    onClose: vi.fn(),
    onReopen: vi.fn(),
    onDelete: vi.fn(),
  });

  expect(screen.getByText('Collected from Greenhouse')).toBeTruthy();
  for (const name of ['Edit', 'Close vacancy', 'Reopen', 'Delete']) {
    expect(screen.queryByRole('button', { name })).toBeNull();
  }
  await fireEvent.click(screen.getByRole('button', { name: 'Analytics' }));
  const report = within(await screen.findByRole('region', { name: 'Analytics' }));
  expect(await report.findByText('7')).toBeTruthy();
  expect(getEmployerVacancyAnalytics).toHaveBeenCalledWith('acme-ats');
});
