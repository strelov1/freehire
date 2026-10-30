import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, like HeaderProfileMenu.test.ts — web/ mounts no components in
// vitest. What is worth pinning: the switcher lists workspaces, never sections (listing
// every section is what made the profile menu long), it reads the gated list rather than
// declaring its own, and it says which workspace you are in.
const SOURCE = readFileSync(join(import.meta.dirname, 'WorkspaceSwitcher.svelte'), 'utf8');

describe('WorkspaceSwitcher', () => {
  it('lists the gated workspaces from the navigation model', () => {
    expect(SOURCE).toContain('visibleWorkspaces(');
    expect(SOURCE).toContain('{#each workspaceList as w (w.id)}');
  });

  it('does not list the individual account sections', () => {
    expect(SOURCE).not.toContain('visibleAccountNav');
    expect(SOURCE).not.toContain('accountNavIcons[');
  });

  it('marks the current workspace, derived from the URL', () => {
    expect(SOURCE).toContain('workspaceFor(page.url.pathname)');
    expect(SOURCE).toContain("aria-current={w.id === current ? 'true' : undefined}");
  });

  it('opens each workspace on its home section', () => {
    expect(SOURCE).toContain('href={resolve(w.home)}');
  });

  it('ends with Settings, the group every workspace shares', () => {
    const lastWorkspace = SOURCE.indexOf('{/each}');
    expect(SOURCE.indexOf("resolve('/my/account')")).toBeGreaterThan(lastWorkspace);
  });
});
