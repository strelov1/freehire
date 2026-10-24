import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import LocationPreferencesFields from './LocationPreferencesFields.svelte';

const { searchCities } = vi.hoisted(() => ({ searchCities: vi.fn() }));

vi.mock('#lib/api.js', () => ({ api: { searchCities } }));

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));

describe('LocationPreferencesFields', () => {
  it('renders its own strings in the resolved locale, leaving facet labels untranslated', () => {
    render(LocationPreferencesFields, {
      props: { value: null, derivedLocation: null, onChange: vi.fn() },
    });

    // The component's own prose renders in Russian.
    expect(screen.getByText('Только для настройки фильтров вакансий — необязательно.')).toBeTruthy();
    expect(screen.getByText('Формат работы')).toBeTruthy();
    expect(screen.getByText('Где вы базируетесь')).toBeTruthy();
    expect(screen.getByPlaceholderText('Город или страна')).toBeTruthy();
    expect(
      screen.getByText('Выберите формат работы выше, чтобы указать, где вы можете работать.'),
    ).toBeTruthy();

    // Facet dictionary values (WORK_MODE_OPTIONS) are untouched by this catalog — they
    // stay in whatever the dictionary itself is written in.
    expect(screen.getByText('Remote')).toBeTruthy();
  });

  it('renders the relocation section strings in Russian once relocation is open', () => {
    // wantsPhysical (onsite/hybrid) gates the relocation block, and relocOpen seeds
    // it expanded — the only way to reach "Open to relocation", "Where you'd
    // relocate", and the "Add a city" picker without simulating a click first.
    render(LocationPreferencesFields, {
      props: {
        value: {
          work_modes: ['onsite'],
          remote: {},
          base: {},
          relocation: { open: true },
        },
        derivedLocation: null,
        onChange: vi.fn(),
      },
    });

    expect(screen.getByText('Готов(а) к переезду')).toBeTruthy();
    expect(screen.getByText('Куда вы готовы переехать (пусто = куда угодно)')).toBeTruthy();
    expect(screen.getByPlaceholderText('Добавить город')).toBeTruthy();
  });

  it('labels the where-based combobox and picks a place with ArrowDown + Enter', async () => {
    searchCities.mockReset().mockResolvedValue([{ value: 'Berlin', country: 'de' }]);
    const onChange = vi.fn();
    render(LocationPreferencesFields, {
      props: { value: null, derivedLocation: null, onChange },
    });

    const input = screen.getByRole('combobox', { name: 'Где вы базируетесь' });
    await fireEvent.input(input, { target: { value: 'Ber' } });
    const option = await screen.findByRole('option', { name: /Berlin/ });

    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(input.getAttribute('aria-activedescendant')).toBe(option.id);

    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ base: expect.objectContaining({ city: 'Berlin', country: 'de' }) }),
    );
  });

  it('closes the where-based list on Escape', async () => {
    searchCities.mockReset().mockResolvedValue([{ value: 'Berlin', country: 'de' }]);
    render(LocationPreferencesFields, {
      props: { value: null, derivedLocation: null, onChange: vi.fn() },
    });

    const input = screen.getByRole('combobox', { name: 'Где вы базируетесь' });
    await fireEvent.input(input, { target: { value: 'Ber' } });
    await screen.findByRole('option', { name: /Berlin/ });

    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).toBeNull();
    expect((input as HTMLInputElement).value).toBe('Ber');
  });
});
