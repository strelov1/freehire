import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: vitest.config.ts runs
// in plain Node, no Svelte plugin, no DOM).
//
// Pins the header-navigation spec's "Paying-tier badge on the desktop profile icon"
// requirement: the badge reads the tier that now rides along on GET /api/v1/auth/me
// (welcome-pro-subscribers), renders only for a paying tier, and lives on the
// desktop-only profile icon rather than anywhere else in the menu.
const SOURCE = readFileSync(join(import.meta.dirname, 'HeaderMenu.svelte'), 'utf8');

describe('HeaderMenu profile icon tier badge', () => {
  it("derives the badge tier from the signed-in user, defaulting to 'free'", () => {
    expect(SOURCE).toContain("currentUser()?.tier ?? 'free'");
  });

  it('shows no badge for a free account', () => {
    expect(SOURCE).toContain("{#if tier !== 'free'}");
  });

  it('attaches the badge to the desktop-only profile icon, not the mobile drawer row', () => {
    // The desktop icon is the `hidden sm:inline-flex` link outside the {#if open} menu
    // panel — the mobile drawer's own Profile row (inside the panel) is a separate
    // element and must not carry a second copy of the badge.
    const desktopIconStart = SOURCE.indexOf("aria-label={tier === 'free' ? 'Profile'");
    const badgeStart = SOURCE.indexOf("{#if tier !== 'free'}");
    const menuPanelStart = SOURCE.indexOf('{#if open}');
    expect(desktopIconStart).toBeGreaterThan(-1);
    expect(badgeStart).toBeGreaterThan(desktopIconStart);
    expect(badgeStart).toBeLessThan(menuPanelStart);
  });

  it("reflects the tier in the icon's accessible name", () => {
    expect(SOURCE).toContain(
      "aria-label={tier === 'free' ? 'Profile' : `Profile (${tier})`}",
    );
  });
});
