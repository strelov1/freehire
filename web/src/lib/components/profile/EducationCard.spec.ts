import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import EducationCard from './EducationCard.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/') },
}));

const { putResumeContacts, getResume } = vi.hoisted(() => ({
  putResumeContacts: vi.fn(),
  getResume: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ api: { putResumeContacts, getResume }, ApiError: class extends Error {} }));

const structured = {
  education: [{ degree: 'BSc', institution: 'FAST' }],
  languages: ['English'],
  certifications: [],
};

beforeEach(() => {
  putResumeContacts.mockReset().mockImplementation((body) => Promise.resolve(body));
  getResume.mockReset().mockResolvedValue(null);
});

describe('EducationCard — autosave', () => {
  it('saves an edited entry when focus leaves it', async () => {
    render(EducationCard, { props: { structured } });
    const degree = screen.getByRole('textbox', { name: 'Degree 1' });

    await fireEvent.input(degree, { target: { value: 'BSc Computer Science' } });
    await fireEvent.focusOut(degree);

    await waitFor(() =>
      expect(putResumeContacts).toHaveBeenCalledWith(
        expect.objectContaining({
          education: [{ degree: 'BSc Computer Science', institution: 'FAST', year: undefined }],
          education_set: true,
        }),
      ),
    );
  });

  it('keeps a new entry out of the saved list until Add', async () => {
    render(EducationCard, { props: { structured } });
    const degree = screen.getByRole('textbox', { name: 'New entry: Degree' });

    await fireEvent.input(degree, { target: { value: 'MSc' } });
    await fireEvent.focusOut(degree);
    expect(putResumeContacts).not.toHaveBeenCalled();

    await fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    await waitFor(() =>
      expect(putResumeContacts).toHaveBeenCalledWith(
        expect.objectContaining({
          education: [
            { degree: 'BSc', institution: 'FAST', year: undefined },
            { degree: 'MSc', institution: '', year: undefined },
          ],
        }),
      ),
    );
  });

  it('saves a removal at once, with an Undo', async () => {
    render(EducationCard, { props: { structured } });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove 1' }));

    await waitFor(() => expect(putResumeContacts).toHaveBeenCalledWith(expect.objectContaining({ education: [] })));
    expect(await screen.findByRole('button', { name: 'Undo' })).toBeTruthy();
  });

  it('saves languages one per line', async () => {
    render(EducationCard, { props: { structured } });
    const box = screen.getByRole('textbox', { name: 'Languages' });

    await fireEvent.input(box, { target: { value: 'English\n Urdu \n' } });
    await fireEvent.focusOut(box);

    await waitFor(() =>
      expect(putResumeContacts).toHaveBeenCalledWith(
        expect.objectContaining({ languages: ['English', 'Urdu'], languages_set: true }),
      ),
    );
  });
});
