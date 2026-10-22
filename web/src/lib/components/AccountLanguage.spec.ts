import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AccountLanguage from './AccountLanguage.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));

const { updateLanguage } = vi.hoisted(() => ({ updateLanguage: vi.fn() }));

vi.mock('#lib/auth.svelte.js', () => ({
  currentUser: () => ({ email: 'a@b.co', language: 'ru' }),
  updateLanguage,
}));

beforeEach(() => {
  updateLanguage.mockReset();
});

describe('AccountLanguage — Russian locale', () => {
  it('renders its heading, description, and the selected language name in Russian', () => {
    render(AccountLanguage);

    expect(screen.getByText('Язык')).toBeTruthy();
    expect(
      screen.getByText(
        'Ваш предпочитаемый язык для ассистента и резюме. Интерфейс аккаунта доступен на английском и русском.',
      ),
    ).toBeTruthy();
    // The combobox's own input value mirrors the selected language's translated name.
    expect(screen.getByDisplayValue('Русский')).toBeTruthy();
  });

  it('renders every language name translated, and the search placeholder, once the list opens', async () => {
    render(AccountLanguage);

    await fireEvent.focus(screen.getByRole('combobox'));

    for (const name of ['Английский', 'Русский', 'Испанский', 'Португальский', 'Немецкий', 'Французский']) {
      expect(screen.getByText(name)).toBeTruthy();
    }
    expect(screen.getByPlaceholderText('Поиск языка…')).toBeTruthy();
  });

  it('renders "no matches" in Russian for a query nothing matches', async () => {
    render(AccountLanguage);

    const input = screen.getByRole('combobox');
    await fireEvent.focus(input);
    await fireEvent.input(input, { target: { value: 'zzz' } });

    expect(screen.getByText('Совпадений нет')).toBeTruthy();
  });

  it('renders the saving and saved states in Russian when picking a different language', async () => {
    let resolveSave: () => void = () => {};
    updateLanguage.mockImplementation(() => new Promise<void>((resolve) => (resolveSave = resolve)));
    render(AccountLanguage);

    await fireEvent.focus(screen.getByRole('combobox'));
    await fireEvent.mouseDown(screen.getByText('Английский'));

    expect(screen.getByText('Сохраняем…')).toBeTruthy();

    resolveSave();
    await vi.waitFor(() => expect(screen.getByText('Сохранено')).toBeTruthy());
  });

  it('renders the save-failure fallback message in Russian for a non-ApiError rejection', async () => {
    updateLanguage.mockRejectedValue(new Error('network blip'));
    render(AccountLanguage);

    await fireEvent.focus(screen.getByRole('combobox'));
    await fireEvent.mouseDown(screen.getByText('Английский'));

    await vi.waitFor(() => expect(screen.getByText('Не удалось сохранить.')).toBeTruthy());
  });
});
