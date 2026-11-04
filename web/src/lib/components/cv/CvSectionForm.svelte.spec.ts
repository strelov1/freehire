import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import { blankExperience, emptyDocument } from '#lib/cv.js';
import type { Document } from '#lib/generated/contracts.js';
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

  it('marks a role as current: the end date gives way to "Present"', async () => {
    render(CvSectionForm, { doc: docWith(['Engineer']), title: 'CV' });
    expect(screen.getByLabelText('Role 1: end')).toBeTruthy();

    await fireEvent.click(screen.getByRole('checkbox', { name: 'I currently work here' }));

    expect(screen.queryByLabelText('Role 1: end')).toBeNull();
    expect(screen.getByText('Present')).toBeTruthy();
  });
});
