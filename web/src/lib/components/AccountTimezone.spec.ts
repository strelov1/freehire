import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import AccountTimezone from './AccountTimezone.svelte';

// Mutable, like SkillsCard.spec.ts's `localeRef` — needed so a test can flip the
// resolved locale mid-render and check that an already-shown message follows it,
// rather than a fixed `{ locale: 'ru' }` object that can only prove the text is
// correct for whichever locale was active at mount time.
const localeRef = vi.hoisted(() => ({ current: 'ru' as 'en' | 'ru' }));
vi.mock('$app/state', () => ({
  page: {
    get data() {
      return { locale: localeRef.current };
    },
    url: new URL('http://localhost/'),
  },
}));

const { updateTimezone, user } = vi.hoisted(() => ({
  updateTimezone: vi.fn(),
  user: { current: { email: 'a@b.co', timezone: 'Europe/Moscow' } as { email: string; timezone?: string } },
}));

vi.mock('#lib/auth.svelte.js', () => ({
  currentUser: () => user.current,
  updateTimezone,
}));

describe('AccountTimezone — Russian locale', () => {
  beforeEach(() => {
    updateTimezone.mockReset();
    user.current = { email: 'a@b.co', timezone: 'Europe/Moscow' };
    localeRef.current = 'ru';
  });

  it('renders its heading, description, and default option in Russian', () => {
    render(AccountTimezone);

    expect(screen.getByText('Часовой пояс')).toBeTruthy();
    expect(
      screen.getByText(
        'Используется, чтобы присылать дайджест по поисковым алертам и учитывать тихие часы по вашему местному времени.',
      ),
    ).toBeTruthy();
  });

  it('renders the unset-timezone placeholder in Russian', () => {
    // No stored timezone AND no detectable one — the same "nothing to seed from"
    // state the component's own `detected` fallback can land in — forces the
    // `{#if !value}` placeholder branch to actually render.
    user.current = { email: 'a@b.co', timezone: undefined };
    const spy = vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(() => {
      throw new Error('no Intl in this test');
    });
    try {
      render(AccountTimezone);
      expect(screen.getByText('Выберите часовой пояс')).toBeTruthy();
    } finally {
      spy.mockRestore();
    }
  });

  it('renders the saving and saved states in Russian', async () => {
    let resolveSave: () => void = () => {};
    updateTimezone.mockImplementation(
      () => new Promise<void>((resolve) => (resolveSave = resolve)),
    );
    render(AccountTimezone);

    const select = screen.getByRole('combobox') as HTMLSelectElement;
    await select.dispatchEvent(new Event('change', { bubbles: true }));
    expect(screen.getByText('Сохраняем…')).toBeTruthy();

    resolveSave();
    await vi.waitFor(() => expect(screen.getByText('Сохранено')).toBeTruthy());
  });

  it('renders the save-failure fallback message in Russian for a non-ApiError rejection', async () => {
    updateTimezone.mockRejectedValue(new Error('network blip'));
    render(AccountTimezone);

    const select = screen.getByRole('combobox') as HTMLSelectElement;
    await select.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => expect(screen.getByText('Не удалось сохранить.')).toBeTruthy());
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});
