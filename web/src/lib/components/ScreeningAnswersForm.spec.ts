import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ScreeningAnswersForm from './ScreeningAnswersForm.svelte';

const page = vi.hoisted(() => ({ data: { locale: 'en' }, url: new URL('http://localhost/') }));
vi.mock('$app/state', () => ({ page }));

const { updateScreeningAnswers, getScreeningAnswers } = vi.hoisted(() => ({
  updateScreeningAnswers: vi.fn(),
  getScreeningAnswers: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({
  api: { updateScreeningAnswers, getScreeningAnswers },
  ApiError: class extends Error {},
}));

beforeEach(() => {
  page.data.locale = 'en';
  updateScreeningAnswers.mockReset().mockImplementation((patch) => Promise.resolve(patch));
  getScreeningAnswers.mockReset().mockResolvedValue(null);
});

describe('ScreeningAnswersForm — autosave', () => {
  it('saves a country picked from the list as its code', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });

    await fireEvent.input(screen.getByRole('textbox', { name: 'Authorized to work in' }), {
      target: { value: 'Germ' },
    });
    await fireEvent.click(screen.getByRole('button', { name: /Germany/ }));

    await waitFor(() => expect(updateScreeningAnswers).toHaveBeenCalledWith({ authorized_countries: ['de'] }));
  });

  it('offers only the countries the server accepts', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });

    // Afghanistan is an ISO country the server's dictionary does not place, so the
    // endpoint would refuse it.
    await fireEvent.input(screen.getByRole('textbox', { name: 'Authorized to work in' }), {
      target: { value: 'Afghan' },
    });

    expect(screen.queryByRole('button', { name: /Afghanistan/ })).toBeNull();
  });

  it('finds a country by its name in the reader\'s language', async () => {
    page.data.locale = 'ru';
    render(ScreeningAnswersForm, { props: { answers: {} } });

    await fireEvent.input(screen.getByRole('textbox', { name: 'Есть разрешение на работу в' }), {
      target: { value: 'Германия' },
    });

    expect(screen.getByRole('button', { name: /Germany/ })).toBeTruthy();
  });

  it('keeps the currency marked while another answer changes', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const currency = screen.getByRole('textbox', { name: 'Currency (ISO 4217)' });

    await fireEvent.input(currency, { target: { value: 'US' } });
    await fireEvent.blur(currency);
    await fireEvent.change(screen.getByRole('combobox', { name: 'Need visa sponsorship?' }), {
      target: { value: 'no' },
    });

    expect(screen.getByText('Currency must be a three-letter code, like USD.')).toBeTruthy();
    expect(updateScreeningAnswers).not.toHaveBeenCalled();
  });

  it('marks a half-typed currency on leaving it and does not save it', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const currency = screen.getByRole('textbox', { name: 'Currency (ISO 4217)' });

    await fireEvent.input(currency, { target: { value: 'US' } });
    await fireEvent.blur(currency);
    await fireEvent.focusOut(currency);

    expect(screen.getByText('Currency must be a three-letter code, like USD.')).toBeTruthy();
    expect(updateScreeningAnswers).not.toHaveBeenCalled();

    await fireEvent.input(currency, { target: { value: 'usd' } });
    await fireEvent.focusOut(currency);
    await waitFor(() => expect(updateScreeningAnswers).toHaveBeenCalledWith({ desired_salary_currency: 'usd' }));
  });

  it('saves a dropdown the moment it is picked', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });

    await fireEvent.change(screen.getByRole('combobox', { name: 'Need visa sponsorship?' }), {
      target: { value: 'no' },
    });

    await waitFor(() => expect(updateScreeningAnswers).toHaveBeenCalledWith({ visa_sponsorship_needed: false }));
    expect(screen.queryByRole('button', { name: /save/i })).toBeNull();
  });

  it('marks a salary that is not a number and does not save it', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const salary = screen.getByRole('textbox', { name: 'Desired salary amount' });

    await fireEvent.input(salary, { target: { value: '12k' } });
    await fireEvent.focusOut(salary);

    expect(screen.getByText('Desired salary amount must be a whole number above zero.')).toBeTruthy();
    expect(salary.getAttribute('aria-invalid')).toBe('true');
    expect(updateScreeningAnswers).not.toHaveBeenCalled();
  });

  it('marks a negative or fractional number the endpoint would refuse', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const notice = screen.getByRole('textbox', { name: 'Notice period (days)' });
    const salary = screen.getByRole('textbox', { name: 'Desired salary amount' });

    await fireEvent.input(notice, { target: { value: '-5' } });
    await fireEvent.focusOut(notice);
    expect(screen.getByText('Notice period (days) must be a whole number.')).toBeTruthy();

    await fireEvent.input(notice, { target: { value: '0' } });
    await fireEvent.input(salary, { target: { value: '1000.5' } });
    await fireEvent.focusOut(salary);
    expect(screen.getByText('Desired salary amount must be a whole number above zero.')).toBeTruthy();
    expect(updateScreeningAnswers).not.toHaveBeenCalled();
  });

  it('refuses hexadecimal or exponent text that Number() would read as a number', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const notice = screen.getByRole('textbox', { name: 'Notice period (days)' });

    for (const value of ['0x10', '1e2']) {
      await fireEvent.input(notice, { target: { value } });
      await fireEvent.focusOut(notice);
      expect(notice.getAttribute('aria-invalid')).toBe('true');
    }
    expect(updateScreeningAnswers).not.toHaveBeenCalled();
  });

  it('marks a comma that does not group by three instead of reading it as thousands', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const salary = screen.getByRole('textbox', { name: 'Desired salary amount' });

    await fireEvent.input(salary, { target: { value: '120,00' } });
    await fireEvent.focusOut(salary);

    expect(salary.getAttribute('aria-invalid')).toBe('true');
    expect(updateScreeningAnswers).not.toHaveBeenCalled();
  });

  it('marks a bad number after the pause, without waiting for focus to leave', async () => {
    vi.useFakeTimers();
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const salary = screen.getByRole('textbox', { name: 'Desired salary amount' });

    await fireEvent.input(salary, { target: { value: '12k' } });
    await vi.advanceTimersByTimeAsync(1100);

    expect(salary.getAttribute('aria-invalid')).toBe('true');
    vi.useRealTimers();
  });

  it('reads a thousands separator as part of the number', async () => {
    render(ScreeningAnswersForm, { props: { answers: {} } });
    const salary = screen.getByRole('textbox', { name: 'Desired salary amount' });

    await fireEvent.input(salary, { target: { value: '120,000' } });
    await fireEvent.focusOut(salary);

    await waitFor(() => expect(updateScreeningAnswers).toHaveBeenCalledWith({ desired_salary_amount: 120000 }));
  });

  it('offers Undo for a changed answer, but not for one filled in for the first time', async () => {
    render(ScreeningAnswersForm, { props: { answers: { notice_period_days: 30 } } });
    const notice = screen.getByRole('textbox', { name: 'Notice period (days)' });

    await fireEvent.input(notice, { target: { value: '60' } });
    await fireEvent.focusOut(notice);
    expect(await screen.findByRole('button', { name: 'Undo' })).toBeTruthy();

    const salary = screen.getByRole('textbox', { name: 'Desired salary amount' });
    await fireEvent.input(salary, { target: { value: '90000' } });
    await fireEvent.focusOut(salary);
    await waitFor(() => expect(updateScreeningAnswers).toHaveBeenCalledTimes(2));
    await screen.findByText('Saved');
    // The salary was blank before, and the endpoint cannot clear it again.
    expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull();
  });
});
