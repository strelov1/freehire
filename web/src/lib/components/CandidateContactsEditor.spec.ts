import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import CandidateContactsEditor from './CandidateContactsEditor.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));

const { putResumeContacts, getResume } = vi.hoisted(() => ({
  putResumeContacts: vi.fn(),
  getResume: vi.fn(),
}));

vi.mock('#lib/api.js', () => ({ api: { putResumeContacts, getResume }, ApiError: class extends Error {} }));

beforeEach(() => {
  putResumeContacts.mockReset().mockImplementation((body) => Promise.resolve(body));
  getResume.mockReset().mockResolvedValue(null);
});

describe('CandidateContactsEditor — Russian locale', () => {
  it('renders every literal string in Russian', () => {
    render(CandidateContactsEditor, { props: {} });

    expect(screen.getByText('Ваши контакты')).toBeTruthy();
    expect(
      screen.getByText(
        'Редактируйте без повторной загрузки. Новый разбор резюме заполняет только пустые поля — он не перезапишет то, что вы ввели.',
      ),
    ).toBeTruthy();
    expect(screen.getByPlaceholderText('Полное имя')).toBeTruthy();
    expect(screen.getByPlaceholderText('Email')).toBeTruthy();
    expect(screen.getByPlaceholderText('Телефон')).toBeTruthy();
    expect(screen.getByPlaceholderText('Местоположение')).toBeTruthy();
    expect(screen.getByText('Ссылки (по одной на строку)')).toBeTruthy();
  });

  it('gives every contact field an accessible name, not just a placeholder', () => {
    render(CandidateContactsEditor, { props: {} });

    for (const name of ['Полное имя', 'Email', 'Телефон', 'Местоположение']) {
      expect(screen.getByRole('textbox', { name })).toBeTruthy();
    }
    expect(screen.getByRole('textbox', { name: 'Ссылки (по одной на строку)' })).toBeTruthy();
  });
});

describe('CandidateContactsEditor — autosave', () => {
  it('saves when focus leaves an edited field, and offers an Undo', async () => {
    render(CandidateContactsEditor, { props: { contacts: { full_name: 'Ada', headline: 'Kept' } } });
    const name = screen.getByRole('textbox', { name: 'Полное имя' });

    await fireEvent.input(name, { target: { value: 'Ada Lovelace' } });
    await fireEvent.focusOut(name);

    await waitFor(() => expect(putResumeContacts).toHaveBeenCalledTimes(1));
    expect(putResumeContacts).toHaveBeenNthCalledWith(1, expect.objectContaining({ full_name: 'Ada Lovelace' }));
    expect(await screen.findByText('Сохранено')).toBeTruthy();

    await fireEvent.click(screen.getByRole('button', { name: 'Отменить' }));
    await waitFor(() => expect(putResumeContacts).toHaveBeenCalledTimes(2));
    expect(putResumeContacts).toHaveBeenNthCalledWith(2, expect.objectContaining({ full_name: 'Ada' }));
  });

  it('saves nothing when focus leaves an untouched field', async () => {
    render(CandidateContactsEditor, { props: { contacts: { full_name: 'Ada' } } });

    await fireEvent.focusOut(screen.getByRole('textbox', { name: 'Полное имя' }));

    expect(putResumeContacts).not.toHaveBeenCalled();
  });

  it('keeps an invalid email on screen, marked, and does not save it', async () => {
    render(CandidateContactsEditor, { props: { contacts: { email: 'ada@example.com' } } });
    const email = screen.getByRole('textbox', { name: 'Email' });

    await fireEvent.input(email, { target: { value: 'ada@' } });
    await fireEvent.focusOut(email);

    expect(screen.getByText('Это не похоже на адрес почты. Он не сохранится, пока не будет исправлен.')).toBeTruthy();
    expect(email.getAttribute('aria-invalid')).toBe('true');
    expect((email as HTMLInputElement).value).toBe('ada@');
    expect(putResumeContacts).not.toHaveBeenCalled();
  });

  it('shows a failed save with a Retry', async () => {
    putResumeContacts.mockRejectedValueOnce(new Error('offline'));
    render(CandidateContactsEditor, { props: { contacts: { phone: '' } } });
    const phone = screen.getByRole('textbox', { name: 'Телефон' });

    await fireEvent.input(phone, { target: { value: '+1 555' } });
    await fireEvent.focusOut(phone);

    expect(await screen.findByText('Не удалось сохранить.')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Повторить' }));
    await waitFor(() => expect(putResumeContacts).toHaveBeenCalledTimes(2));
    expect(await screen.findByText('Сохранено')).toBeTruthy();
  });
});
