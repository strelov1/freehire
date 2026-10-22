import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WebhookConfig } from '#lib/types.js';
import { must } from '#lib/utils.js';
import WebhookSettingsView from './WebhookSettingsView.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));

const { getWebhook, createOrUpdateWebhook, setWebhookEnabled, deleteWebhook, testWebhook } = vi.hoisted(
  () => ({
    getWebhook: vi.fn(),
    createOrUpdateWebhook: vi.fn(),
    setWebhookEnabled: vi.fn(),
    deleteWebhook: vi.fn(),
    testWebhook: vi.fn(),
  }),
);

const { StubApiError } = vi.hoisted(() => ({
  StubApiError: class StubApiError extends Error {
    constructor(
      public status: number,
      message = 'failed',
    ) {
      super(message);
    }
  },
}));

vi.mock('#lib/api.js', () => ({
  api: { getWebhook, createOrUpdateWebhook, setWebhookEnabled, deleteWebhook, testWebhook },
  ApiError: StubApiError,
}));

vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));

const webhook: WebhookConfig = {
  url: 'https://example.com/hook',
  enabled: true,
  created_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
  last_success_at: null,
  disabled_at: null,
};

beforeEach(() => {
  getWebhook.mockReset();
  createOrUpdateWebhook.mockReset();
  setWebhookEnabled.mockReset();
  deleteWebhook.mockReset();
  testWebhook.mockReset();
});

describe('WebhookSettingsView — Russian locale', () => {
  it('renders the heading, description, form, and "create" state in Russian when no webhook exists', async () => {
    getWebhook.mockResolvedValue(null);
    render(WebhookSettingsView);

    // Wait for the loading skeleton to resolve into the actual form before
    // asserting on it.
    await vi.waitFor(() => expect(screen.getByText('Создать вебхук')).toBeTruthy());
    expect(screen.getByText('Вебхук')).toBeTruthy();
    expect(
      screen.getByText(
        'Получайте HTTP POST при каждом новом совпадении по сохранённому поиску — вместе с email/Telegram или вместо них. Включите его для сохранённого поиска в настройках алерта, когда здесь будет настроен адрес.',
      ),
    ).toBeTruthy();
    expect(screen.getByText('URL')).toBeTruthy();
    expect(screen.getByPlaceholderText('https://example.com/freehire-hook')).toBeTruthy();
  });

  it('renders the enabled status line, action buttons, and delete dialog in Russian once a webhook exists', async () => {
    getWebhook.mockResolvedValue(webhook);
    render(WebhookSettingsView);

    await vi.waitFor(() => expect(screen.getByText(/Включён/)).toBeTruthy());
    expect(screen.getByText('Отключить')).toBeTruthy();
    // "Удалить" labels both the page's own Delete button and the confirm
    // dialog's confirm button — the page's is the first of the two.
    const pageDelete = must(screen.getAllByText('Удалить')[0], 'page delete button');

    await fireEvent.click(pageDelete);
    expect(screen.getByText('Удалить вебхук?')).toBeTruthy();
    expect(
      screen.getByText('Сохранённые поиски, подписанные на этот канал, сразу перестанут получать уведомления.'),
    ).toBeTruthy();
  });

  it('renders the invalid-URL form error in Russian', async () => {
    getWebhook.mockResolvedValue(null);
    createOrUpdateWebhook.mockRejectedValue(new StubApiError(400));
    render(WebhookSettingsView);

    const input = await vi.waitFor(() =>
      screen.getByPlaceholderText('https://example.com/freehire-hook'),
    );
    // Syntactically valid (so the native `type="url"` constraint lets the form
    // submit) — the mocked API is what rejects it, exactly as the server-side
    // 400 this branch actually handles would.
    await fireEvent.input(input, { target: { value: 'https://example.com' } });
    await fireEvent.click(screen.getByText('Создать вебхук'));

    await vi.waitFor(() =>
      expect(screen.getByText('Введите корректный URL, начинающийся с http:// или https://.')).toBeTruthy(),
    );
  });

  it('shows what the destination answered when saving fails the test delivery', async () => {
    getWebhook.mockResolvedValue(null);
    createOrUpdateWebhook.mockRejectedValue(
      new StubApiError(422, 'webhook test failed: destination responded 404'),
    );
    render(WebhookSettingsView);

    const input = await vi.waitFor(() =>
      screen.getByPlaceholderText('https://example.com/freehire-hook'),
    );
    await fireEvent.input(input, { target: { value: 'https://example.com/hook' } });
    await fireEvent.click(screen.getByText('Создать вебхук'));

    await vi.waitFor(() =>
      expect(
        screen.getByText('Адрес не принял тестовый запрос: destination responded 404'),
      ).toBeTruthy(),
    );
  });

  it('sends a test delivery from the button and reports the result', async () => {
    getWebhook.mockResolvedValue(webhook);
    testWebhook.mockResolvedValueOnce({ ...webhook, last_success_at: new Date().toISOString() });
    render(WebhookSettingsView);

    await fireEvent.click(await vi.waitFor(() => screen.getByText('Проверить')));
    await vi.waitFor(() => expect(screen.getByText('Тестовый запрос доставлен.')).toBeTruthy());
    expect(testWebhook).toHaveBeenCalledTimes(1);

    testWebhook.mockRejectedValueOnce(
      new StubApiError(422, 'webhook test failed: destination responded 400'),
    );
    await fireEvent.click(screen.getByText('Проверить'));
    await vi.waitFor(() =>
      expect(
        screen.getByText('Адрес не принял тестовый запрос: destination responded 400'),
      ).toBeTruthy(),
    );
    expect(screen.queryByText('Тестовый запрос доставлен.')).toBeNull();
  });
});
