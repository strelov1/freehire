import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Alert from './alert.svelte';
import type { AlertVariant } from './alert.svelte';
import { must } from './test-utils';

const slot = (html: string) => createRawSnippet(() => ({ render: () => html }));

function setup(variant?: AlertVariant, live?: boolean) {
  const { container } = render(Alert, { variant, live, children: slot('<span>Heads up</span>') });
  return must(container.querySelector('div'));
}

describe('Alert', () => {
  // role="alert" is an assertive live region — it interrupts the screen reader
  // mid-sentence. Only the failure variant has earned that.
  it('interrupts only for the destructive variant', () => {
    expect(setup('destructive').getAttribute('role')).toBe('alert');
  });

  it('stays quiet for the informational variants', () => {
    expect(setup().getAttribute('role')).toBeNull();
    expect(setup('default').getAttribute('role')).toBeNull();
    expect(setup('brand').getAttribute('role')).toBeNull();
  });

  // A caller that announces the failure itself must not have it read twice.
  it('stays quiet when the caller voices the failure itself', () => {
    const alert = setup('destructive', false);
    expect(alert.getAttribute('role')).toBeNull();
    expect(alert.textContent).toContain('Heads up');
  });

  it('renders what it was given either way', () => {
    expect(setup('destructive').textContent).toContain('Heads up');
  });
});
