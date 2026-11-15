import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import AccountLayout from './+layout.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/profile/cv-readiness') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => null }));
vi.mock('#lib/assistant/AssistantChat.svelte', async () => ({
  default: (await import('../../../vitest-stubs/EmptyComponent.svelte')).default,
}));

// jsdom has no scrolling; keep the real drawer and its focus management.
const scrollIntoView = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView');
beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: () => {} });
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
});
afterAll(() => {
  vi.restoreAllMocks();
  if (scrollIntoView) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', scrollIntoView);
  else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView');
});

describe('account shell controls', () => {
  it.each(['Loading…', 'Could not load your profile.', 'No CV uploaded.'])(
    'keeps navigation available when a page shows %s without a heading',
    async (content) => {
      const children = createRawSnippet(() => ({ render: () => `<p>${content}</p>` }));
      render(AccountLayout, { props: { children } });

      expect(screen.getByText(content)).toBeTruthy();
      const navigation = screen.getByRole('button', { name: /Account sections:/ });
      await fireEvent.click(navigation);
      expect(navigation.getAttribute('aria-expanded')).toBe('true');
      expect(within(screen.getByRole('dialog')).getByRole('link', { name: 'Profile' })).toBeTruthy();
    },
  );
});
