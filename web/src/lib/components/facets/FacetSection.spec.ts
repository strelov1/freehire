import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { FacetDef, FacetStore } from '#lib/facets.js';
import FacetSection from './FacetSection.svelte';

vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'ru' }));
const store: FacetStore = {
  facet: () => ({ include: [], exclude: [], matchAll: false }),
  cycle: vi.fn(), pick: vi.fn(), toggleSign: vi.fn(), add: vi.fn(), remove: vi.fn(),
  clearFacet: vi.fn(), setMatchAll: vi.fn(),
};

describe('localized facet search', () => {
  it('translates a registry-provided placeholder and accessible label', () => {
    const def: FacetDef = {
      param: 'skills', label: 'Skills', control: 'select',
      excludable: false, placeholder: 'Search skills', options: [],
    };
    render(FacetSection, { def, store });
    expect(screen.getByPlaceholderText('Поиск навыков')).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Поиск навыков' })).toBeTruthy();
  });

  it('uses the localized generic prompt for an unknown facet', () => {
    const def: FacetDef = {
      param: 'custom', label: 'Custom', control: 'select',
      excludable: false, placeholder: 'Search custom terms', options: [],
    };
    render(FacetSection, { def, store });
    expect(screen.getByPlaceholderText('Поиск вариантов')).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Поиск вариантов' })).toBeTruthy();
  });
});
