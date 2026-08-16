import { defineMessages } from '$lib/i18n/t';

// Nav item labels are keyed by href (not nested per-section) so a lookup can fall
// back to accountNav.ts's own `item.label` for any href this catalog hasn't
// caught up with, instead of the whole section silently disappearing.
export const messages = defineMessages(
  {
    navItems: {
      '/my/profile': 'Profile',
      '/my/activity': 'Activity',
      '/my/tracking': 'Tracking',
      '/my/inbox': 'Inbox',
      '/my/market-pulse': 'Market Pulse',
      '/my/assistant': 'Agent',
      '/my/cvs': 'Tailor',
      '/my/referrals': 'Referrals',
      '/my/notifications': 'Notifications',
      '/my/api-keys': 'API keys',
      '/my/submissions': 'My submissions',
      '/my/contributions': 'Contributions',
      '/my/credits': 'Credits',
      '/my/security': 'Security',
    },
    shell: {
      signInPrompt: 'Sign in to access your account.',
      signIn: 'Sign in',
      expandSidebar: 'Expand sidebar',
      collapseSidebar: 'Collapse sidebar',
      accountSections: 'Account sections',
    },
    rail: {
      menu: 'Menu',
      closeMenu: 'Close menu',
    },
  },
  {
    navItems: {
      '/my/profile': 'Профиль',
      '/my/activity': 'Активность',
      '/my/tracking': 'Трекинг',
      '/my/inbox': 'Входящие',
      '/my/market-pulse': 'Пульс рынка',
      '/my/assistant': 'Агент',
      '/my/cvs': 'Адаптация CV',
      '/my/referrals': 'Рефералы',
      '/my/notifications': 'Уведомления',
      '/my/api-keys': 'API-ключи',
      '/my/submissions': 'Мои публикации',
      '/my/contributions': 'Вклад',
      '/my/credits': 'Кредиты',
      '/my/security': 'Безопасность',
    },
    shell: {
      signInPrompt: 'Войдите, чтобы получить доступ к аккаунту.',
      signIn: 'Войти',
      expandSidebar: 'Развернуть панель',
      collapseSidebar: 'Свернуть панель',
      accountSections: 'Разделы аккаунта',
    },
    rail: {
      menu: 'Меню',
      closeMenu: 'Закрыть меню',
    },
  },
);

/** A nav item's translated label, falling back to its English default (the
 *  literal from accountNav.ts) if this catalog has no entry for its href. */
export function navLabel(
  resolved: { navItems: Record<string, string> },
  href: string,
  fallback: string,
): string {
  return resolved.navItems[href] ?? fallback;
}
