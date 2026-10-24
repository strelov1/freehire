import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import CandidateContactsEditor from './CandidateContactsEditor.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));

const { putResumeContacts } = vi.hoisted(() => ({ putResumeContacts: vi.fn() }));

vi.mock('#lib/api.js', () => ({ api: { putResumeContacts } }));

beforeEach(() => {
  putResumeContacts.mockReset();
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
    expect(screen.getByText('Сохранить контакты')).toBeTruthy();
  });

  it('gives every contact field an accessible name, not just a placeholder', () => {
    render(CandidateContactsEditor, { props: {} });

    for (const name of ['Полное имя', 'Email', 'Телефон', 'Местоположение']) {
      expect(screen.getByRole('textbox', { name })).toBeTruthy();
    }
    expect(screen.getByRole('textbox', { name: 'Ссылки (по одной на строку)' })).toBeTruthy();
  });

  it('renders the save-success note in Russian', async () => {
    putResumeContacts.mockResolvedValue({});
    render(CandidateContactsEditor, { props: {} });

    await fireEvent.click(screen.getByText('Сохранить контакты'));

    expect(screen.getByText('Контакты сохранены.')).toBeTruthy();
  });

  it('renders the save-failure fallback message in Russian for a non-Error rejection', async () => {
    // The component's own fallback string only fires when the rejection isn't an
    // Error instance (see CandidateContactsEditor.svelte's catch) — a real Error's
    // own .message is used verbatim and is not a translation concern.
    putResumeContacts.mockRejectedValue('network blip');
    render(CandidateContactsEditor, { props: {} });

    await fireEvent.click(screen.getByText('Сохранить контакты'));

    expect(screen.getByText('Не удалось сохранить контакты.')).toBeTruthy();
  });
});
