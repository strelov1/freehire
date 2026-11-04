import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import { blankExperience, emptyDocument } from '#lib/cv.js';
import { type Document, MaxExperience } from '#lib/generated/contracts.js';
import CvSectionForm from './CvSectionForm.svelte';

// $state, as the workspace holds it: the form edits the bound document's rows in place, which
// only re-renders through a reactive proxy.
const docWith = (roles: string[]): Document => {
  const doc = $state({
    ...emptyDocument(),
    experience: roles.map((role) => ({ ...blankExperience(), role })),
  });
  return doc;
};

const roleValues = () =>
  screen.getAllByPlaceholderText('Role').map((el) => (el as HTMLInputElement).value);

describe('CvSectionForm', () => {
  it('adds a new role at the top of the list, not the bottom', async () => {
    render(CvSectionForm, { doc: docWith(['Older role']), title: 'CV' });
    await fireEvent.click(screen.getByRole('button', { name: /Add role/ }));
    expect(roleValues()).toEqual(['', 'Older role']);
  });

  it('refuses an Add at the section limit instead of letting the save drop the oldest role', async () => {
    const roles = Array.from({ length: MaxExperience }, (_, i) => `Role ${i + 1}`);
    render(CvSectionForm, { doc: docWith(roles), title: 'CV' });
    const add = screen.getByRole('button', { name: /Add role/ }) as HTMLButtonElement;
    expect(add.disabled).toBe(true);
    await fireEvent.click(add);
    expect(roleValues()).toEqual(roles);
  });

  it('marks a role as current: the end date gives way to "Present"', async () => {
    render(CvSectionForm, { doc: docWith(['Engineer']), title: 'CV' });
    expect(screen.getByLabelText('Role 1: end')).toBeTruthy();

    await fireEvent.click(screen.getByRole('checkbox', { name: 'I currently work here' }));

    expect(screen.queryByLabelText('Role 1: end')).toBeNull();
    expect(screen.getByText('Present')).toBeTruthy();
  });
});
