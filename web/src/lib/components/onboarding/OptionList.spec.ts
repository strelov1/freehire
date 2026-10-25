import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import OptionList from './OptionList.svelte';

const options = [
  { value: 'a', label: 'Alpha' },
  { value: 'b', label: 'Bravo' },
  { value: 'c', label: 'Charlie' },
];

function setup(value: string | null = null) {
  const onSelect = vi.fn();
  render(OptionList, { options, value, onSelect, label: 'Pick one' });
  const radios = screen.getAllByRole('radio') as [HTMLElement, HTMLElement, HTMLElement];
  return { onSelect, radios };
}

describe('OptionList — one tab stop, arrows move and select', () => {
  it('makes only the first option tabbable when nothing is checked', () => {
    const { radios } = setup();
    expect(radios.map((r) => r.tabIndex)).toEqual([0, -1, -1]);
  });

  it('makes only the checked option tabbable', () => {
    const { radios } = setup('b');
    expect(radios.map((r) => r.tabIndex)).toEqual([-1, 0, -1]);
    expect(radios[1].getAttribute('aria-checked')).toBe('true');
  });

  it('moves focus and selection to the next option on ArrowDown and ArrowRight', async () => {
    const { onSelect, radios } = setup('a');
    radios[0].focus();
    await fireEvent.keyDown(radios[0], { key: 'ArrowDown' });
    expect(onSelect).toHaveBeenLastCalledWith('b');
    expect(document.activeElement).toBe(radios[1]);
    await fireEvent.keyDown(radios[1], { key: 'ArrowRight' });
    expect(onSelect).toHaveBeenLastCalledWith('c');
    expect(document.activeElement).toBe(radios[2]);
  });

  it('wraps around at both ends with ArrowUp/ArrowLeft and ArrowDown', async () => {
    const { onSelect, radios } = setup('a');
    await fireEvent.keyDown(radios[0], { key: 'ArrowUp' });
    expect(onSelect).toHaveBeenLastCalledWith('c');
    expect(document.activeElement).toBe(radios[2]);
    await fireEvent.keyDown(radios[2], { key: 'ArrowDown' });
    expect(onSelect).toHaveBeenLastCalledWith('a');
    await fireEvent.keyDown(radios[0], { key: 'ArrowLeft' });
    expect(onSelect).toHaveBeenLastCalledWith('c');
  });

  it('jumps to the first and last option with Home and End', async () => {
    const { onSelect, radios } = setup('b');
    await fireEvent.keyDown(radios[1], { key: 'End' });
    expect(onSelect).toHaveBeenLastCalledWith('c');
    expect(document.activeElement).toBe(radios[2]);
    await fireEvent.keyDown(radios[2], { key: 'Home' });
    expect(onSelect).toHaveBeenLastCalledWith('a');
    expect(document.activeElement).toBe(radios[0]);
  });

  it('still selects on click and ignores unrelated keys', async () => {
    const { onSelect, radios } = setup();
    await fireEvent.click(radios[2]);
    expect(onSelect).toHaveBeenCalledWith('c');
    onSelect.mockClear();
    await fireEvent.keyDown(radios[0], { key: 'a' });
    expect(onSelect).not.toHaveBeenCalled();
  });
});
