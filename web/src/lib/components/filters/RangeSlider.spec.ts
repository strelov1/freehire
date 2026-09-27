import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import RangeSlider from './RangeSlider.svelte';

// The two thumbs are separate native inputs, so nothing but this component's handlers
// stops one being dragged past the other — and a lower bound above the upper one is a
// search that matches nothing while the control looks like an ordinary range.

function setup(lo: number, hi: number) {
  const onLo = vi.fn();
  const onHi = vi.fn();
  render(RangeSlider, {
    props: { min: 0, max: 300000, step: 5000, lo, hi, loLabel: 'Minimum salary', hiLabel: 'Maximum salary', onLo, onHi },
  });
  const loInput = screen.getByRole('slider', { name: 'Minimum salary' }) as HTMLInputElement;
  const hiInput = screen.getByRole('slider', { name: 'Maximum salary' }) as HTMLInputElement;
  return { onLo, onHi, loInput, hiInput };
}

describe('RangeSlider', () => {
  it('reports each thumb through its own callback', async () => {
    const { onLo, onHi, loInput, hiInput } = setup(0, 300000);
    await fireEvent.input(loInput, { target: { value: '100000' } });
    await fireEvent.input(hiInput, { target: { value: '150000' } });
    expect(onLo).toHaveBeenCalledWith(100000);
    expect(onHi).toHaveBeenCalledWith(150000);
  });

  it('stops the lower thumb one step short of the upper', async () => {
    const { onLo, loInput } = setup(0, 150000);
    await fireEvent.input(loInput, { target: { value: '200000' } });
    expect(onLo).toHaveBeenCalledWith(145000);
    // The thumb itself must snap back too: the browser has already drawn it where the
    // pointer went, and a prop that did not change will not redraw it.
    expect(loInput.value).toBe('145000');
  });

  it('stops the upper thumb one step short of the lower', async () => {
    const { onHi, hiInput } = setup(100000, 300000);
    await fireEvent.input(hiInput, { target: { value: '50000' } });
    expect(onHi).toHaveBeenCalledWith(105000);
    expect(hiInput.value).toBe('105000');
  });
});
