import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import OverlayFixture from './fixtures/OverlayFixture.svelte';

// Svelte detaches an {#if} branch's DOM before it runs the branch's attachment
// teardowns, so by the time focusTrap/menuNav clean up, the element that had focus is
// already gone and focus sits on <body>. These specs mount real overlays to hold the
// restore to that order, which no pure-function test can see.
describe.each(['trap', 'menu'] as const)('focus restore (%s)', (kind) => {
  it('returns focus to the trigger when the overlay closes from inside', async () => {
    render(OverlayFixture, { kind });
    const trigger = screen.getByRole('button', { name: 'Open' });
    trigger.focus();
    await fireEvent.click(trigger);
    const inner = screen.getByRole(kind === 'trap' ? 'button' : 'menuitem', { name: kind === 'trap' ? 'Inner' : 'Item' });
    inner.focus();
    await fireEvent.click(inner);
    expect(document.activeElement).toBe(trigger);
  });

  it('leaves focus alone when the app already moved it elsewhere', async () => {
    render(OverlayFixture, { kind });
    const trigger = screen.getByRole('button', { name: 'Open' });
    trigger.focus();
    await fireEvent.click(trigger);
    const outside = screen.getByRole('button', { name: 'Close from outside' });
    outside.focus();
    await fireEvent.click(outside);
    expect(document.activeElement).toBe(outside);
  });
});
