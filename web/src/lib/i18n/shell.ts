import { defineMessages } from '#lib/i18n/t.js';

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
      '/my/watchlist': 'Watchlist',
      '/my/inbox': 'Inbox',
      '/my/market-pulse': 'Market Pulse',
      '/my/assistant': 'Agent',
      '/my/cvs': 'Tailor CV',
      '/my/talent-network': 'Talent Network',
      '/my/referrals': 'Referral requests',
      '/my/referrals/offers': 'Offers to refer',
      '/my/referrals/incoming': 'Incoming requests',
      '/buddies': 'Interview buddies',
      '/my/mentorship': 'Mentorship',
      '/my/mentorship/bookings': 'Booked with you',
      '/my/mentorship/schedule': 'Schedule',
      '/my/mentorship/profile': 'Mentor profile',
      '/my/api': 'API',
      '/my/notifications': 'Notifications',
      '/my/integrations': 'Integrations',
      '/my/contributions': 'Contributions',
      '/my/employer': 'Company profile',
      '/my/employer/vacancies': 'Vacancies',
      '/my/plan': 'Plan',
      '/my/account': 'Account',
    },
    shell: {
      expandSidebar: 'Expand sidebar',
      collapseSidebar: 'Collapse sidebar',
      accountSections: 'Account sections',
      settings: 'Settings',
    },
    // Keyed by WorkspaceId / NavGroupId; accountNav.ts carries no English copy of these.
    workspaces: {
      seeker: 'My job search',
      buddies: 'Interview buddies',
      mentor: 'Mentor',
      referrer: 'Referrer',
      employer: 'Employer',
      api: 'API',
    },
    // `main` has no heading: it is the workspace itself.
    navGroups: {
      network: 'Network',
      settings: 'Settings',
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
        '/my/watchlist': 'Избранные компании',
        '/my/inbox': 'Входящие',
        '/my/market-pulse': 'Пульс рынка',
        '/my/assistant': 'Агент',
        '/my/cvs': 'Адаптация резюме',
        '/my/talent-network': 'Сеть талантов',
        '/my/referrals': 'Запросы рекомендаций',
        '/my/referrals/offers': 'Готов рекомендовать',
        '/my/referrals/incoming': 'Входящие запросы',
        '/buddies': 'Бадди для интервью',
        '/my/mentorship': 'Менторство',
        '/my/mentorship/bookings': 'Записи к вам',
        '/my/mentorship/schedule': 'Расписание',
        '/my/mentorship/profile': 'Профиль ментора',
        '/my/api': 'API',
        '/my/notifications': 'Уведомления',
        '/my/integrations': 'Интеграции',
        // Vacancies the user submitted for review — not posts they published.
        // The section is "Contribute a board": paste a job link so we crawl a
        // board we don't have yet. A literal "Вклад" says nothing about that.
        '/my/contributions': 'Вклад',
        '/my/employer': 'Профиль компании',
        '/my/employer/vacancies': 'Вакансии',
        '/my/plan': 'Тариф',
        // "Пригласить друга" and not "Рефералы": that word is taken by the
        // employee-referral marketplace above, and two sections sharing a name
        // is a menu nobody can navigate.
        '/my/account': 'Аккаунт',
      },
      shell: {
        expandSidebar: 'Развернуть панель',
        collapseSidebar: 'Свернуть панель',
        accountSections: 'Разделы аккаунта',
        settings: 'Настройки',
      },
      workspaces: {
        seeker: 'Мой поиск работы',
        buddies: 'Бадди для интервью',
        mentor: 'Ментор',
        referrer: 'Рекомендатель',
        employer: 'Работодатель',
        api: 'API',
      },
      navGroups: {
        network: 'Сеть',
        settings: 'Настройки',
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
export function navLabel(resolved: { navItems: Record<string, string> }, href: string, fallback: string): string {
  return resolved.navItems[href] ?? fallback;
}
