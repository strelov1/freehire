import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { JobList } from '#lib/types.js';
import JobListsView from './JobListsView.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/my/lists') },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));

const { ensureLoaded, create, remove } = vi.hoisted(() => ({
  ensureLoaded: vi.fn(),
  create: vi.fn(),
  remove: vi.fn(),
}));

const itemsRef = { current: [] as JobList[] };

vi.mock('#lib/jobLists.svelte.js', () => ({
  jobLists: {
    get items() {
      return itemsRef.current;
    },
    ensureLoaded,
    reset: vi.fn(),
    create,
    update: vi.fn(),
    share: vi.fn(),
    unshare: vi.fn(),
    remove,
  },
}));

const oneJob: JobList = {
  id: 1,
  name: 'Backend roles',
  description: '',
  public_slug: '',
  job_count: 1,
  created_at: null,
  updated_at: null,
};

const fiveJobs: JobList = { ...oneJob, id: 2, name: 'Frontend roles', job_count: 5 };

beforeEach(() => {
  ensureLoaded.mockReset().mockResolvedValue(undefined);
  create.mockReset();
  remove.mockReset();
  itemsRef.current = [];
});

describe('JobListsView — Russian locale', () => {
  it('renders the heading, description, and empty state in Russian', async () => {
    render(JobListsView);

    expect(screen.getByText('Списки вакансий')).toBeTruthy();
    expect(
      screen.getByText(
        'Группируйте отдельные вакансии в именованные списки — независимо от звезды «Сохранить» — и, при желании, делитесь любым из них как публичной страницей только для чтения.',
      ),
    ).toBeTruthy();
    // Wait for the loading skeleton to resolve into the empty state before
    // asserting on it.
    await vi.waitFor(() =>
      expect(
        screen.getByText('Пока нет списков вакансий. Создайте один или добавьте вакансию в новый список с её карточки.'),
      ).toBeTruthy(),
    );
    expect(screen.getByText('Новый список')).toBeTruthy();
  });

  it('renders the create form in Russian', async () => {
    render(JobListsView);
    await vi.waitFor(() => expect(screen.getByText('Новый список')).toBeTruthy());

    await fireEvent.click(screen.getByText('Новый список'));

    expect(screen.getByPlaceholderText('Название списка')).toBeTruthy();
    expect(screen.getByPlaceholderText('Описание (необязательно)')).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Название списка' })).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Описание (необязательно)' })).toBeTruthy();
    expect(screen.getByText('Создать список')).toBeTruthy();
    expect(screen.getByText('Отмена')).toBeTruthy();
  });

  it('renders the correct Russian plural form for the job count', async () => {
    itemsRef.current = [oneJob, fiveJobs];
    render(JobListsView);

    await vi.waitFor(() => expect(screen.getByText('Backend roles')).toBeTruthy());
    expect(screen.getByText(/^1 вакансия$/)).toBeTruthy();
    expect(screen.getByText(/^5 вакансий$/)).toBeTruthy();
  });

  it('renders the delete-confirmation dialog with the list name interpolated, in Russian', async () => {
    itemsRef.current = [oneJob];
    render(JobListsView);

    await vi.waitFor(() => expect(screen.getByText('Backend roles')).toBeTruthy());
    await fireEvent.click(screen.getByTitle('Удалить'));

    expect(screen.getByText('Удалить список вакансий «Backend roles»?')).toBeTruthy();
  });

  it('renders the create-failure fallback message in Russian for a non-ApiError rejection', async () => {
    create.mockRejectedValue(new Error('network blip'));
    render(JobListsView);
    await vi.waitFor(() => expect(screen.getByText('Новый список')).toBeTruthy());

    await fireEvent.click(screen.getByText('Новый список'));
    await fireEvent.input(screen.getByPlaceholderText('Название списка'), {
      target: { value: 'Test list' },
    });
    await fireEvent.click(screen.getByText('Создать список'));

    await vi.waitFor(() =>
      expect(screen.getByText('Не удалось создать этот список. Попробуйте ещё раз.')).toBeTruthy(),
    );
  });
});
