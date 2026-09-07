import { GITHUB_URL } from './github.svelte';

// External profile links shared by every surface that shows the GitHub / LinkedIn /
// Telegram / Discord row (Footer, the /signin brand panel) — one place so the URLs
// can't drift between them the way jobs.company_slug_aliases exists precisely
// because a fact like this once lived in more than one place and disagreed.
/** The community invite. Exported because the header menu offers the same room the
 *  footer's socials row does, and it used to carry its own copy of the URL — the
 *  drift this file's own comment warns about, one import away from being real. */
export const DISCORD_URL = 'https://discord.gg/Cghjh3dA5N';

export const SOCIAL_LINKS: { provider: string; label: string; href: string }[] = [
  { provider: 'github', label: 'GitHub', href: GITHUB_URL },
  { provider: 'linkedin', label: 'LinkedIn', href: 'https://linkedin.com/company/freehire-dev/' },
  { provider: 'telegram', label: 'Telegram', href: 'https://t.me/freehiredev' },
  { provider: 'discord', label: 'Discord', href: DISCORD_URL },
];
