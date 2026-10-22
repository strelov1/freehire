import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CompanyListItem, ExperienceEmploymentWithAtoms } from '#lib/types.js';
import ExperienceEmploymentCard from './ExperienceEmploymentCard.svelte';

// The inline edit form gains a company-catalogue autocomplete for job-kind employments
// only — project-kind entries name a project, not a company — see openspec change
// experience-company-picker-present-checkbox.

const { listCompanies } = vi.hoisted(() => ({ listCompanies: vi.fn() }));

vi.mock('#lib/api.js', () => ({ api: { listCompanies } }));

const ringCentral: CompanyListItem = {
  slug: 'ringcentral',
  name: 'RingCentral',
  job_count: 57,
  feedback_count: 0,
  feedback_rating_avg: null,
};

const jobEmployment: ExperienceEmploymentWithAtoms = {
  id: 'e1',
  kind: 'job',
  company: 'Acme',
  role: 'Engineer',
  atoms: [],
};

const currentJobEmployment: ExperienceEmploymentWithAtoms = {
  ...jobEmployment,
  id: 'e3',
  current: true,
};

const projectEmployment: ExperienceEmploymentWithAtoms = {
  id: 'e2',
  kind: 'project',
  name: 'Side Project',
  atoms: [],
};

function baseProps(employment: ExperienceEmploymentWithAtoms) {
  return {
    employment,
    selectedIds: [],
    busy: false,
    turnActive: false,
    onToggleSelect: vi.fn(),
    onConfirmAtom: vi.fn(),
    onSaveAtomEdit: vi.fn(),
    onSavePromote: vi.fn(),
    onRemoveAtom: vi.fn(),
    onSaveEmployment: vi.fn(),
    onRemoveEmployment: vi.fn(),
  };
}

beforeEach(() => {
  listCompanies.mockReset().mockResolvedValue({ items: [ringCentral], hasMore: false });
});

describe('ExperienceEmploymentCard edit form', () => {
  it('offers catalogue suggestions for a job-kind Company field', async () => {
    render(ExperienceEmploymentCard, { props: baseProps(jobEmployment) });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.input(screen.getByLabelText('Company'), { target: { value: 'ring' } });

    expect(await screen.findByRole('option', { name: /RingCentral/ })).toBeTruthy();
  });

  it('keeps a plain text input with no suggestions for a project-kind name', async () => {
    render(ExperienceEmploymentCard, { props: baseProps(projectEmployment) });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.input(screen.getByLabelText('Project name'), {
      target: { value: 'ring' },
    });

    expect(listCompanies).not.toHaveBeenCalled();
    expect(screen.queryByRole('listbox')).toBeNull();
  });

  it('pre-checks "I currently work here" and hides the End date for an already-current job', async () => {
    render(ExperienceEmploymentCard, { props: baseProps(currentJobEmployment) });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

    expect(screen.getByLabelText<HTMLInputElement>('I currently work here').checked).toBe(true);
    expect(screen.queryByPlaceholderText('End')).toBeNull();
  });

  it('unchecking restores the End date and saves as not current', async () => {
    const onSaveEmployment = vi.fn().mockResolvedValue(true);
    render(ExperienceEmploymentCard, {
      props: { ...baseProps(currentJobEmployment), onSaveEmployment },
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.click(screen.getByLabelText('I currently work here'));

    expect(screen.getByPlaceholderText('End')).toBeTruthy();

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveEmployment).toHaveBeenCalledWith(
      currentJobEmployment,
      expect.objectContaining({ current: false }),
    );
  });

  it('checking hides the End date and saves as current', async () => {
    const onSaveEmployment = vi.fn().mockResolvedValue(true);
    render(ExperienceEmploymentCard, { props: { ...baseProps(jobEmployment), onSaveEmployment } });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.click(screen.getByLabelText('I currently work here'));

    expect(screen.queryByPlaceholderText('End')).toBeNull();

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveEmployment).toHaveBeenCalledWith(
      jobEmployment,
      expect.objectContaining({ current: true, end: undefined }),
    );
  });

  it('has no "I currently work here" control for a project-kind entry', async () => {
    render(ExperienceEmploymentCard, { props: baseProps(projectEmployment) });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

    expect(screen.queryByLabelText('I currently work here')).toBeNull();
  });
});
