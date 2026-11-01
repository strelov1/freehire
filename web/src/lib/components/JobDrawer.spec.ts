import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { MyJob } from '#lib/types.js';
import JobDrawer from './JobDrawer.svelte';

// The owner's report was a phone in the Russian interface, so the drawer renders in `ru`.
vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/my/tracking'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), pushState: vi.fn(), replaceState: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true, currentUser: () => ({ id: 'u1' }) }));
vi.mock('easymde', () => ({
  default: class {
    codemirror = { on: () => {}, getInputField: () => document.createElement('textarea') };
    value() {
      return '';
    }
    toTextArea() {}
  },
}));

const { getTrackedApplication } = vi.hoisted(() => ({ getTrackedApplication: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getTrackedApplication }, isPlanRefusal: () => false }));

const job = {
  public_slug: 'staff-cloud-native-software-engineer-acme',
  title: 'Staff Cloud Native Software Engineer',
  company: 'Acme',
  seniority: 'staff',
} as unknown as NonNullable<MyJob['job']>;

const item: MyJob = {
  id: 'app-1',
  company_slug: 'acme',
  role_title: 'Staff Cloud Native Software Engineer',
  job,
  viewed_at: '2026-10-01T00:00:00Z',
  saved_at: '2026-10-01T00:00:00Z',
  applied_at: null,
  stage: 'preparing',
  notes: null,
  email_count: 0,
  last_activity_at: null,
  days_silent: null,
  silence_state: null,
  followed_up_at: null,
  cv_opened_at: null,
};

const noop = () => {};
const props = (over: Partial<MyJob> = {}) => ({
  item: { ...item, ...over },
  pendingOutcome: false,
  onsetstage: noop,
  onsavenotes: async () => {},
  onchooseoutcome: noop,
  onautoapplyreview: noop,
  onremove: async () => {},
  onclose: noop,
  onfollowup: noop,
});

beforeEach(() => {
  getTrackedApplication.mockReset().mockResolvedValue({
    job,
    emails: [],
    events: [
      {
        id: 1,
        kind: 'stage_set',
        signal: 'preparing',
        source: 'user',
        observed: true,
        occurred_at: new Date().toISOString(),
        company_slug: 'acme',
      },
    ],
  });
});

describe('JobDrawer — phone layout in Russian', () => {
  it('gives the title its own row, away from the employer and the close button', () => {
    render(JobDrawer, props());

    const heading = screen.getByRole('heading', { level: 2 });
    expect(heading.textContent).toBe('Staff Cloud Native Software Engineer');
    // The close button shares the employer's row, not the title's.
    const close = screen.getByRole('button', { name: 'Закрыть' });
    expect(heading.contains(close)).toBe(false);
    expect(close.parentElement?.textContent).toContain('Acme');
    expect(heading.parentElement?.contains(close.parentElement as Node)).toBe(true);
    expect(close.parentElement?.contains(heading)).toBe(false);
  });

  it('names the stage in Russian in the badge, the history and the selector', async () => {
    render(JobDrawer, props());

    // The badge beside the facet pills.
    expect(screen.getAllByText('Подготовка').some((el) => el.tagName === 'SPAN')).toBe(true);
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    const options = within(select).getAllByRole('option').map((o) => o.textContent);
    expect(options).toContain('Собеседование');
    expect(options).toContain('Отказ');
    expect(options).not.toContain('Interview');
    const groups = Array.from(select.querySelectorAll('optgroup')).map((g) => g.label);
    expect(groups).toContain('Закрыто');
    expect(select.value).toBe('preparing');
    const historyTab = screen.getAllByRole('tab').find((tab) => tab.id === 'jobdrawer-tab-history');
    if (!historyTab) throw new Error('Missing History tab');
    await fireEvent.click(historyTab);
    // The history row interpolates the label, not the raw `preparing` key.
    await waitFor(() => expect(screen.getByText('Переход на этап «Подготовка»')).toBeTruthy());
    expect(screen.queryByText(/preparing/)).toBeNull();


  });

  it('keeps View job in the employer row and Tailor CV in the action row', () => {
    render(JobDrawer, props());

    const viewJob = screen.getByRole('link', { name: 'Открыть вакансию' });
    expect(viewJob.parentElement?.textContent).toContain('Acme');
    expect(viewJob.getAttribute('target')).toBe('_blank');
    // The one framed call to action, while the application is still being prepared.
    const tailor = screen.getByRole('button', { name: 'Адаптировать CV' });
    expect(tailor.className).toContain('bg-brand');
    expect(tailor.parentElement).not.toBe(viewJob.parentElement);
  });

  it('weighs every action the same once the application has been sent', () => {
    render(JobDrawer, props({ applied_at: '2026-10-02T00:00:00Z', stage: 'applied' }));

    expect(screen.getByRole('button', { name: 'Адаптировать CV' }).className).not.toContain('bg-brand');
    expect(screen.getAllByText('Отклик отправлен').length).toBeGreaterThan(0);
  });

  it('keeps the tab strip a keyboard tablist inside a horizontal scroller', () => {
    render(JobDrawer, props());

    const tablist = screen.getByRole('tablist');
    expect(tablist.parentElement?.className).toContain('overflow-x-auto');
    const tabs = within(tablist).getAllByRole('tab');
    expect(tabs.map((t) => t.textContent?.trim())).toContain('Описание');
    expect(tabs.filter((t) => t.tabIndex === 0)).toHaveLength(1);
  });
});
