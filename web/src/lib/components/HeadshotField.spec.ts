import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import HeadshotField from './HeadshotField.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));

const { getPhoto, putPhoto, testLocale } = vi.hoisted(() => ({
  testLocale: { value: 'en' as 'en' | 'ru' },
  getPhoto: vi.fn(),
  putPhoto: vi.fn(),
}));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => testLocale.value }));
vi.mock('#lib/api.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#lib/api.js')>();
  return { ...actual, api: { ...actual.api, getPhoto, putPhoto } };
});

beforeEach(() => {
  testLocale.value = 'en';
  getPhoto.mockReset();
  putPhoto.mockReset();
});

describe('HeadshotField', () => {
  it('uses the account language for photo labels and errors', async () => {
    testLocale.value = 'ru';
    getPhoto.mockRejectedValue(new Error('offline'));
    render(HeadshotField);
    expect(await screen.findByRole('button', { name: 'Повторить' })).toBeTruthy();
    expect(screen.getByText('Не удалось загрузить информацию о фото.')).toBeTruthy();
  });

  it('offers recovery after a failed metadata request and restores upload on retry', async () => {
    getPhoto.mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ enabled: true, present: false });
    render(HeadshotField);

    await fireEvent.click(await screen.findByRole('button', { name: 'Try again' }));

    expect(await screen.findByRole('button', { name: 'Upload photo' })).toBeTruthy();
    expect(screen.queryByText('Could not load your photo.')).toBeNull();
    expect(getPhoto).toHaveBeenCalledTimes(2);
  });

  it('hides upload when storage is explicitly disabled', async () => {
    getPhoto.mockResolvedValue({ enabled: false, present: false });
    render(HeadshotField);
    await vi.waitFor(() => expect(screen.queryByRole('status')).toBeNull());
    expect(screen.queryByRole('button', { name: 'Upload photo' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
  });

  it('opens the picker from the avatar and rejects an oversized file before uploading', async () => {
    getPhoto.mockResolvedValue({ enabled: true, present: false });
    const { container } = render(HeadshotField);
    const avatar = await screen.findByRole('button', { name: 'Upload photo' });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const click = vi.spyOn(input, 'click');
    await fireEvent.click(avatar);
    expect(click).toHaveBeenCalledOnce();

    const file = new File(['image'], 'photo.png', { type: 'image/png' });
    Object.defineProperty(file, 'size', { value: 7 * 1024 * 1024 });
    await fireEvent.change(input, { target: { files: [file] } });

    expect(await screen.findByText('Choose an image up to 6 MB.')).toBeTruthy();
    expect(putPhoto).not.toHaveBeenCalled();
  });
});
