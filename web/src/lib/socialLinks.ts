import { GITHUB_URL } from './github.svelte';

// External profile links shared by every surface that shows the GitHub / LinkedIn /
// Telegram / Discord row (Footer, the /signin brand panel) — one place so the URLs
// can't drift between them the way jobs.company_slug_aliases exists precisely
// because a fact like this once lived in more than one place and disagreed.
export const SOCIAL_LINKS: { provider: string; label: string; href: string }[] = [
  { provider: 'github', label: 'GitHub', href: GITHUB_URL },
  { provider: 'linkedin', label: 'LinkedIn', href: 'https://linkedin.com/company/freehire-dev/' },
  { provider: 'telegram', label: 'Telegram', href: 'https://t.me/freehiredev' },
  { provider: 'discord', label: 'Discord', href: 'https://discord.gg/sYnZksswR' },
];
