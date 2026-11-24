import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('/recruiters/job-promotion', () => {
  it('opens a private email draft with the entered details and preserves URL query parameters', async () => {
    const assign = vi.fn<(url: string) => void>();
    vi.stubGlobal('location', { assign });
    render(Page);

    const email = 'hiring+pilot@company.com';
    const vacancy = 'https://company.com/careers/role?source=web&team=data#apply';
    await fireEvent.input(screen.getByRole('textbox', { name: 'Work email' }), { target: { value: email } });
    await fireEvent.input(screen.getByRole('textbox', { name: 'Vacancy URL' }), { target: { value: vacancy } });
    await fireEvent.click(screen.getByRole('button', { name: 'Open email draft' }));

    expect(assign).toHaveBeenCalledOnce();
    const draft = new URL(assign.mock.lastCall?.[0] ?? '');
    expect(draft.protocol).toBe('mailto:');
    expect(draft.pathname).toBe('hello@freehire.me');
    expect(draft.searchParams.get('subject')).toBe('Job promotion pilot');
    expect(draft.searchParams.get('body')).toBe(`Work email: ${email}\r\nVacancy URL: ${vacancy}`);
  });
});
