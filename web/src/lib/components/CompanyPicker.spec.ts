import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CompanyListItem } from '#lib/types.js';
import CompanyPicker from './CompanyPicker.svelte';
import CompanyPickerTestHarness from './CompanyPickerTestHarness.svelte';

// Generalizing CompanyPicker for the experience bank (see openspec change
// experience-company-picker-present-checkbox): its typed text becomes an optional
// bindable `value` prop, in addition to the existing `onSelect` callback, so a consumer
// whose field is always free text (no catalogue slug) can track it too — without
// changing anything for the two existing consumers (ReferralsView, MentorProfileEditor)
// that only ever cared about a resolved company.

const { listCompanies } = vi.hoisted(() => ({ listCompanies: vi.fn() }));

vi.mock('#lib/api.js', () => ({ api: { listCompanies } }));

const ringCentral: CompanyListItem = {
  slug: 'ringcentral',
  name: 'RingCentral',
  job_count: 57,
  feedback_count: 0,
  feedback_rating_avg: null,
};

beforeEach(() => {
  listCompanies.mockReset().mockResolvedValue({ items: [ringCentral], hasMore: false });
});

describe('CompanyPicker', () => {
  it('exposes the typed text through a bound value prop before anything is picked', async () => {
    render(CompanyPickerTestHarness, { props: { onSelect: vi.fn() } });

    await fireEvent.input(screen.getByPlaceholderText('Search your company…'), {
      target: { value: 'ring' },
    });

    expect(screen.getByTestId('picker-value').textContent).toBe('ring');
  });

  it('sets the bound value to the canonical name and notifies onSelect when a suggestion is picked', async () => {
    const onSelect = vi.fn();
    render(CompanyPickerTestHarness, { props: { onSelect } });

    await fireEvent.input(screen.getByPlaceholderText('Search your company…'), {
      target: { value: 'ring' },
    });
    const option = await screen.findByRole('option', { name: /RingCentral/ });
    await fireEvent.mouseDown(option);

    expect(screen.getByTestId('picker-value').textContent).toBe('RingCentral');
    expect(onSelect).toHaveBeenCalledWith({ slug: 'ringcentral', name: 'RingCentral' });
  });

  it('still works with only onSelect and no bound value, exactly like today', async () => {
    const onSelect = vi.fn();
    render(CompanyPicker, { props: { onSelect } });

    await fireEvent.input(screen.getByPlaceholderText('Search your company…'), {
      target: { value: 'ring' },
    });
    const option = await screen.findByRole('option', { name: /RingCentral/ });
    await fireEvent.mouseDown(option);

    await waitFor(() => expect(onSelect).toHaveBeenCalledWith({ slug: 'ringcentral', name: 'RingCentral' }));
  });

  it('picks with the keyboard: ArrowDown highlights through aria-activedescendant, Enter picks', async () => {
    const onSelect = vi.fn();
    render(CompanyPickerTestHarness, { props: { onSelect } });

    const input = screen.getByRole('combobox');
    await fireEvent.input(input, { target: { value: 'ring' } });
    const option = await screen.findByRole('option', { name: /RingCentral/ });
    // Rows stay out of the Tab order; the input owns the keyboard.
    expect(option.getAttribute('tabindex')).toBe('-1');

    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(input.getAttribute('aria-activedescendant')).toBe(option.id);
    expect(option.getAttribute('aria-selected')).toBe('true');

    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(onSelect).toHaveBeenCalledWith({ slug: 'ringcentral', name: 'RingCentral' });
    expect(screen.getByTestId('picker-value').textContent).toBe('RingCentral');
    expect(screen.queryByRole('listbox')).toBeNull();
  });

  it('closes the list on Escape without picking or clearing the typed text', async () => {
    const onSelect = vi.fn();
    render(CompanyPickerTestHarness, { props: { onSelect } });

    const input = screen.getByRole('combobox');
    await fireEvent.input(input, { target: { value: 'ring' } });
    await screen.findByRole('option', { name: /RingCentral/ });

    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(input.getAttribute('aria-expanded')).toBe('false');
    expect(onSelect).not.toHaveBeenCalledWith(expect.objectContaining({ slug: 'ringcentral' }));
    expect(screen.getByTestId('picker-value').textContent).toBe('ring');
  });

  it('gives each instance its own listbox id', async () => {
    render(CompanyPicker, { props: { id: 'a' } });
    render(CompanyPicker, { props: { id: 'b' } });

    const ids = screen.getAllByRole('combobox').map((el) => el.getAttribute('aria-controls'));
    expect(ids).toHaveLength(2);
    expect(ids[0]).not.toBe(ids[1]);
  });
});
