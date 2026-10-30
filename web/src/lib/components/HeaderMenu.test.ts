import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: no Svelte
// plugin, no DOM in vitest.config.ts).
//
// openspec/changes/split-header-profile-menu split the desktop profile/account
// items and Log out off into HeaderProfileMenu.svelte, leaving this component's
// own dropdown as site nav + theme toggle on desktop. The mobile drawer is
// unchanged — same markup, now scoped with `sm:hidden` rather than removed.
const SOURCE = readFileSync(join(import.meta.dirname, 'HeaderMenu.svelte'), 'utf8');

describe('HeaderMenu desktop/mobile split', () => {
  it('renders HeaderProfileMenu in place of the old inline profile/sign-in block', () => {
    expect(SOURCE).toContain("import HeaderProfileMenu from './HeaderProfileMenu.svelte'");
    expect(SOURCE).toContain('<HeaderProfileMenu');
  });

  it('does not render its own inline profile icon or sign-in button in the bar', () => {
    // The bar-level profile/sign-in block this used to own (aria-label="Profile" /
    // aria-label="Sign in" as a direct <a>/<button> in the control strip) is now
    // HeaderProfileMenu's job.
    // Anchored on role="menu" (the dropdown panel), not the literal string
    // "{#if open}" — that also occurs earlier inside a code comment (the
    // menu-toggle button's own doc comment), which would truncate the "bar"
    // slice before reaching the end of the bar's actual markup.
    const bar = SOURCE.slice(0, SOURCE.indexOf('role="menu"'));
    expect(bar).not.toContain('aria-label="Profile"');
    expect(bar).not.toContain('aria-label="Sign in"');
  });

  // The same switcher the desktop profile menu renders, so a phone and a laptop offer
  // the same way into each workspace.
  it('renders the workspace switcher, not the section list, for a signed-in phone user', () => {
    const mobile = SOURCE.slice(SOURCE.indexOf('<div class="sm:hidden">'));
    expect(mobile.indexOf('<WorkspaceSwitcher')).toBeLessThan(mobile.indexOf("resolve('/submit')"));
    expect(SOURCE).not.toContain('visibleAccountNav');
  });

  it('scopes the account-items block to mobile with sm:hidden', () => {
    expect(SOURCE).toMatch(/<div class="sm:hidden">(\s*<!--[\s\S]*?-->)?\s*\{#if isAuthenticated\(\)\}/);
  });

  it('drops the desktop auth action but keeps the desktop theme toggle', () => {
    const start = SOURCE.indexOf('hidden sm:block');
    const end = SOURCE.indexOf('Mobile-only: GitHub + theme + auth');
    const desktopTailBlock = SOURCE.slice(start, end);
    expect(desktopTailBlock).toContain('{@render themeButton()}');
    expect(desktopTailBlock).not.toContain('{@render authButton()}');
  });

  it('still renders the auth action in the mobile pinned bottom bar', () => {
    const mobileBar = SOURCE.slice(SOURCE.indexOf('Mobile-only: GitHub + theme + auth'));
    expect(mobileBar).toContain('{@render authButton()}');
  });

  it('renders Open immediately after About in the nav links', () => {
    const aboutAt = SOURCE.indexOf('NAV.about.href');
    const openAt = SOURCE.indexOf('NAV.open.href', aboutAt);
    expect(openAt, 'expected an Open link after the About link').toBeGreaterThan(aboutAt);
    const between = SOURCE.slice(SOURCE.indexOf('</a>', aboutAt), openAt);
    // Nothing but whitespace/comments/attributes between the two menuitems — no
    // other nav link sits between them.
    expect(between).not.toContain('role="menuitem"');
  });
});
