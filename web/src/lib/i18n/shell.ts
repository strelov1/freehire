import { defineMessages } from '$lib/i18n/t';

// Nav item labels are keyed by href (not nested per-section) so a lookup can fall
// back to accountNav.ts's own `item.label` for any href this catalog hasn't
// caught up with, instead of the whole section silently disappearing. That
// fallback is deliberately quiet, so shell.test.ts asserts the coverage the
// fallback would otherwise hide — three sections had drifted before it existed.
export const messages = defineMessages(
  {
    navItems: {
      '/my/profile': 'Profile',
      '/my/activity': 'Activity',
      '/my/tracking': 'Tracking',
      '/my/inbox': 'Inbox',
      '/my/lists': 'Job lists',
      '/my/market-pulse': 'Market Pulse',
      '/my/assistant': 'Agent',
      '/my/cvs': 'Tailor',
      '/my/referrals': 'Referrals',
      '/my/notifications': 'Notifications',
      '/my/integrations': 'Integrations',
      '/my/api-keys': 'API keys',
      '/my/webhook': 'Webhook',
      '/my/submissions': 'My submissions',
      '/my/contributions': 'Contributions',
      '/my/plan': 'Plan',
      '/my/security': 'Security',
    },
    shell: {
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
    ru: {
      navItems: {
        '/my/profile': 'Профиль',
        '/my/activity': 'Активность',
        '/my/tracking': 'Трекинг',
        '/my/inbox': 'Входящие',
        '/my/lists': 'Списки вакансий',
        '/my/market-pulse': 'Пульс рынка',
        '/my/assistant': 'Агент',
        '/my/cvs': 'Адаптация CV',
        '/my/referrals': 'Рефералы',
        '/my/notifications': 'Уведомления',
        '/my/integrations': 'Интеграции',
        '/my/api-keys': 'API-ключи',
        '/my/webhook': 'Вебхук',
        // Vacancies the user submitted for review — not posts they published.
        '/my/submissions': 'Мои вакансии',
        // The section is "Contribute a board": paste a job link so we crawl a
        // board we don't have yet. A literal "Вклад" says nothing about that.
        '/my/contributions': 'Добавить борд',
        '/my/plan': 'Тариф',
        '/my/security': 'Безопасность',
      },
      shell: {
        expandSidebar: 'Развернуть панель',
        collapseSidebar: 'Свернуть панель',
        accountSections: 'Разделы аккаунта',
      },
      rail: {
        menu: 'Меню',
        closeMenu: 'Закрыть меню',
      },
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
