import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EmployerCompany } from '#lib/types.js';
import EmployerDashboard from './EmployerDashboard.svelte';

const { listEmployerVacancies, prefillEmployerVacancy, searchCities } = vi.hoisted(() => ({
  listEmployerVacancies: vi.fn(),
  prefillEmployerVacancy: vi.fn(),
  searchCities: vi.fn(),
}));

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('#lib/api.js', () => ({
  api: { listEmployerVacancies, prefillEmployerVacancy, searchCities },
  ApiError: class extends Error {},
}));

const company = { company_name: 'Acme', role: 'holder' } as unknown as EmployerCompany;

async function openExternalForm() {
  render(EmployerDashboard, { company, tab: 'vacancies', onProfileSaved: vi.fn(), onLeft: vi.fn() });
  await fireEvent.click(await screen.findByRole('button', { name: /New vacancy/ }));
  await fireEvent.click(screen.getByRole('radio', { name: /On my own site/ }));
}

describe('EmployerDashboard link prefill', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listEmployerVacancies.mockResolvedValue([]);
  });

  it('fills only the empty fields, turning the HTML description into markdown', async () => {
    prefillEmployerVacancy.mockResolvedValue({
      title: 'Payroll Specialist',
      location: 'Austin, United States',
      work_mode: 'remote',
      seniority: 'senior',
      description: '<p>Run <strong>payroll</strong>.</p>',
      existing_job_slug: 'payroll-specialist-ab12cd34',
    });
    await openExternalForm();
    const title = screen.getByLabelText(/^Title/) as HTMLInputElement;
    await fireEvent.input(title, { target: { value: 'My own title' } });
    await fireEvent.input(screen.getByPlaceholderText('https://…'), { target: { value: 'https://x.test/jobs/1' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Fill in from this link' }));

    expect(prefillEmployerVacancy).toHaveBeenCalledWith('https://x.test/jobs/1');
    expect(await screen.findByDisplayValue('Austin, United States')).toBeTruthy();
    expect(title.value).toBe('My own title');
    expect((screen.getByLabelText(/Work mode/) as HTMLSelectElement).value).toBe('remote');
    expect(screen.getByDisplayValue('Run **payroll**.')).toBeTruthy();
  });

  it('says so when the link yields nothing', async () => {
    prefillEmployerVacancy.mockResolvedValue({});
    await openExternalForm();
    await fireEvent.input(screen.getByPlaceholderText('https://…'), { target: { value: 'https://x.test/none' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Fill in from this link' }));
    expect(await screen.findByText('Nothing to fill in from this link.')).toBeTruthy();
  });

  it('drops an answer that arrives after the URL was changed', async () => {
    let finish: (v: unknown) => void = () => {};
    prefillEmployerVacancy.mockReturnValue(new Promise((r) => (finish = r)));
    await openExternalForm();
    const urlInput = screen.getByPlaceholderText('https://…');
    await fireEvent.input(urlInput, { target: { value: 'https://x.test/old' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Fill in from this link' }));
    await fireEvent.input(urlInput, { target: { value: 'https://x.test/new' } });
    finish({ title: 'Stale title' });
    await tick();
    expect((screen.getByLabelText(/^Title/) as HTMLInputElement).value).toBe('');
  });
});
