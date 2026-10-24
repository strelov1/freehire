import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { FacetOption } from '#lib/facets.js';
import RemoteSearchSelect from './RemoteSearchSelect.svelte';

// The picker behind the skills, avoid-list and company facets. It must be reachable
// from the keyboard: the input owns focus, the arrows walk the options through
// aria-activedescendant, Enter picks, Escape closes the floating list.

const options: FacetOption[] = [
  { value: 'go', label: 'Go', count: 10 },
  { value: 'rust', label: 'Rust', count: 5 },
];

function setup(props: Partial<{ expand: boolean; include: string[] }> = {}) {
  const onToggle = vi.fn();
  render(RemoteSearchSelect, {
    props: {
      search: () => Promise.resolve(options),
      include: [],
      onToggle,
      fallbackLabel: (v: string) => v,
      label: 'Search skills',
      ...props,
    },
  });
  const inputs = screen.getAllByRole('combobox', { name: 'Search skills' });
  return { onToggle, input: inputs[inputs.length - 1] as HTMLElement };
}

describe('RemoteSearchSelect', () => {
  it('picks with the keyboard: ArrowDown highlights through aria-activedescendant, Enter picks', async () => {
    const { onToggle, input } = setup();

    await fireEvent.focus(input);
    const go = await screen.findByRole('option', { name: /Go/ });
    // Rows stay out of the Tab order; the input owns the keyboard.
    expect(go.getAttribute('tabindex')).toBe('-1');
    expect(input.getAttribute('aria-controls')).toBe(screen.getByRole('listbox').id);

    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(input.getAttribute('aria-activedescendant')).toBe(go.id);
    expect(go.getAttribute('aria-selected')).toBe('true');

    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    const rust = screen.getByRole('option', { name: /Rust/ });
    expect(input.getAttribute('aria-activedescendant')).toBe(rust.id);

    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(onToggle).toHaveBeenCalledWith('rust');
  });

  it('closes the floating list on Escape without picking', async () => {
    const { onToggle, input } = setup();

    await fireEvent.focus(input);
    await screen.findByRole('option', { name: /Go/ });
    await fireEvent.keyDown(input, { key: 'ArrowDown' });

    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(input.getAttribute('aria-expanded')).toBe('false');
    expect(input.getAttribute('aria-activedescendant')).toBeNull();
    expect(onToggle).not.toHaveBeenCalled();
  });

  it('still picks with the mouse', async () => {
    const { onToggle, input } = setup();

    await fireEvent.focus(input);
    await fireEvent.mouseDown(await screen.findByRole('option', { name: /Go/ }));
    expect(onToggle).toHaveBeenCalledWith('go');
  });

  it('marks the empty-state row as a disabled option', async () => {
    const { input } = setup({ include: ['go', 'rust'] });

    await fireEvent.focus(input);
    const status = await screen.findByRole('option', { name: 'No results' });
    expect(status.getAttribute('aria-disabled')).toBe('true');
    expect(status.getAttribute('aria-selected')).toBe('false');
  });

  it('keeps the inline list of expand mode inside a labelled listbox the input controls', async () => {
    const { onToggle, input } = setup({ expand: true });

    const go = await screen.findByRole('option', { name: /Go/ });
    const list = screen.getByRole('listbox', { name: 'Search skills' });
    expect(list.contains(go)).toBe(true);
    expect(input.getAttribute('aria-controls')).toBe(list.id);
    expect(input.getAttribute('aria-expanded')).toBe('true');

    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(input.getAttribute('aria-activedescendant')).toBe(go.id);
    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(onToggle).toHaveBeenCalledWith('go');
  });

  it('gives each instance its own listbox id', async () => {
    setup();
    setup();

    const ids = screen.getAllByRole('combobox').map((el) => el.getAttribute('aria-controls'));
    expect(ids).toHaveLength(2);
    expect(ids[0]).not.toBe(ids[1]);
  });
});
