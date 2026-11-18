import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import {
  blankCertification,
  blankEducation,
  blankExperience,
  blankLanguage,
  blankProject,
  blankSkillGroup,
  emptyDocument,
} from '#lib/cv.js';
import {
  type Document,
  MaxCertifications,
  MaxEducation,
  MaxExperience,
  MaxLanguages,
  MaxProjects,
  MaxSkillGroups,
} from '#lib/generated/contracts.js';
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

// Every section the form can grow, with the ceiling the server truncates it to. One row per
// section, so a section added to the form without a cap shows up as a missing case here.
const sections = [
  { key: 'experience', max: MaxExperience, make: blankExperience, add: /Add role/ },
  { key: 'education', max: MaxEducation, make: blankEducation, add: /Add education/ },
  { key: 'skills', max: MaxSkillGroups, make: blankSkillGroup, add: /Add group/ },
  { key: 'languages', max: MaxLanguages, make: blankLanguage, add: /Add language/ },
  { key: 'projects', max: MaxProjects, make: blankProject, add: /Add project/ },
  { key: 'certifications', max: MaxCertifications, make: blankCertification, add: /Add certification/ },
] as const;

const fullDoc = (key: string, rows: unknown[]): Document => {
  const doc = $state({ ...emptyDocument(), [key]: rows } as Document);
  return doc;
};

const roleValues = () => screen.getAllByPlaceholderText('Role').map((el) => (el as HTMLInputElement).value);

describe('CvSectionForm', () => {
  it('adds a new role at the top of the list, not the bottom', async () => {
    render(CvSectionForm, { doc: docWith(['Older role']), title: 'CV' });
    await fireEvent.click(screen.getByRole('button', { name: /Add role/ }));
    expect(roleValues()).toEqual(['', 'Older role']);
  });

  it('refuses an Add at the section limit instead of letting the save drop the oldest role', async () => {
    const roles = Array.from({ length: MaxExperience }, (_, i) => `Role ${i + 1}`);
    render(CvSectionForm, { doc: docWith(roles), title: 'CV' });
    await fireEvent.click(screen.getByRole('button', { name: /Add role/ }));
    expect(roleValues()).toEqual(roles);
  });

  it.each(sections)('disables Add once $key is at its ceiling ($max)', ({ key, max, make, add }) => {
    render(CvSectionForm, {
      doc: fullDoc(
        key,
        Array.from({ length: max }, () => make()),
      ),
      title: 'CV',
    });
    expect((screen.getByRole('button', { name: add }) as HTMLButtonElement).disabled).toBe(true);
  });

  it.each(sections)('keeps Add enabled for $key below its ceiling', ({ key, max, make, add }) => {
    render(CvSectionForm, {
      doc: fullDoc(
        key,
        Array.from({ length: max - 1 }, () => make()),
      ),
      title: 'CV',
    });
    expect((screen.getByRole('button', { name: add }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('marks a role as current: the end date gives way to "Present"', async () => {
    render(CvSectionForm, { doc: docWith(['Engineer']), title: 'CV' });
    expect(screen.getByLabelText('Role 1: end')).toBeTruthy();

    await fireEvent.click(screen.getByRole('checkbox', { name: 'I currently work here' }));

    expect(screen.queryByLabelText('Role 1: end')).toBeNull();
    expect(screen.getByText('Present')).toBeTruthy();
  });
});
