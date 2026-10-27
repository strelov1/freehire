import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: no Svelte
// plugin, no DOM in vitest.config.ts).
//
// This is the desktop-only profile menu split off HeaderMenu's consolidated
// dropdown (openspec/changes/split-header-profile-menu). What is worth pinning:
// the panel's item order (Log out must land where it is reachable without
// scrolling — the whole reason for the split), that it reuses the same
// overlay-coordination and close behaviors HeaderMenu already has, and that a
// signed-out visitor gets a direct action rather than an empty dropdown.
const SOURCE = readFileSync(join(import.meta.dirname, 'HeaderProfileMenu.svelte'), 'utf8');

describe('HeaderProfileMenu', () => {
  it('renders the profile icon as a dropdown trigger', () => {
    expect(SOURCE).toContain('aria-haspopup="menu"');
    expect(SOURCE).toContain('aria-expanded={open}');
  });

  it('lists the panel items in order: Profile, account links, Submit a job, Moderation, Log out', () => {
    const order = [
      "resolve('/my/profile')",
      '{#each accountLinks as link',
      "resolve('/submit')",
      'Submit a job',
      "resolve('/moderation')",
      'Moderation',
      'Log out',
    ];
    let cursor = -1;
    for (const needle of order) {
      const at = SOURCE.indexOf(needle, cursor + 1);
      expect(at, `expected to find "${needle}" after position ${cursor}`).toBeGreaterThan(cursor);
      cursor = at;
    }
  });

  it('gates Moderation on the moderator role', () => {
    expect(SOURCE).toContain('{#if isModerator}');
  });

  it('imports the shared account-links data instead of redeclaring it', () => {
    expect(SOURCE).toContain("import { accountLinks } from '#lib/headerAccountLinks.js'");
  });

  it('closes on outside click, Escape, and navigation', () => {
    expect(SOURCE).toContain('root.contains(e.target as Node)) open = false');
    expect(SOURCE).toContain("e.key === 'Escape'");
    expect(SOURCE).toContain('afterNavigate(');
  });

  it('coordinates with the other header overlays', () => {
    expect(SOURCE).toContain("import { openedOverlay, closedOverlay } from '#lib/headerOverlay.js'");
    expect(SOURCE).toContain('openedOverlay(closeSelf)');
    expect(SOURCE).toContain('closedOverlay(closeSelf)');
  });

  it('shows a direct, labelled Sign-in action with no dropdown when signed out', () => {
    const signedOutBranch = SOURCE.slice(SOURCE.indexOf('{:else}'));
    expect(signedOutBranch).toMatch(/\/>\s*Sign in\s*<\/button>/);
    expect(signedOutBranch).toContain('onclick={signIn}');
    expect(signedOutBranch).not.toContain('aria-haspopup="menu"');
  });

  // A first-time visitor on a phone had no sign-in control outside the menu: the whole
  // component was hidden below sm. Only the signed-in profile trigger stays desktop-only.
  it('hides itself on a phone only when signed in', () => {
    expect(SOURCE).toContain("isAuthenticated() ? 'hidden sm:inline-flex' : 'inline-flex'");
  });
});

// Pins the header-navigation spec's "Paying-tier badge on the desktop profile icon"
// requirement (welcome-pro-subscribers): the badge reads the tier that rides along on
// GET /api/v1/auth/me, renders only for a paying tier, and lives on the trigger button
// this component owns — it moved here from HeaderMenu.svelte along with the rest of the
// profile control (split-header-profile-menu).
describe('HeaderProfileMenu tier badge', () => {
  it("derives the badge tier from the signed-in user, defaulting to 'free'", () => {
    expect(SOURCE).toContain("currentUser()?.tier ?? 'free'");
  });

  it('shows no badge for a free account', () => {
    expect(SOURCE).toContain("{#if tier !== 'free'}");
  });

  it('attaches the badge to the trigger button, not the dropdown panel', () => {
    const triggerStart = SOURCE.indexOf("aria-label={tier === 'free' ? 'Profile'");
    const badgeStart = SOURCE.indexOf("{#if tier !== 'free'}");
    const menuPanelStart = SOURCE.indexOf('{#if open}');
    expect(triggerStart).toBeGreaterThan(-1);
    expect(badgeStart).toBeGreaterThan(triggerStart);
    expect(badgeStart).toBeLessThan(menuPanelStart);
  });

  it("reflects the tier in the trigger's accessible name", () => {
    expect(SOURCE).toContain("aria-label={tier === 'free' ? 'Profile' : `Profile (${tier})`}");
  });
});
