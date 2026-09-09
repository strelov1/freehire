import { describe, expect, it } from 'vitest';
import { previewMentorSlug, resolveMentorSlugForSubmit } from './mentorSlugPreview';

// Mirrors internal/identity/username.Sanitize (plus the same word-join the backend does
// before calling it) closely enough that what a mentor sees while typing their name
// matches what SubmitProfile will actually derive — see profile.go's SubmitProfile.
describe('previewMentorSlug', () => {
  it('joins words with a hyphen and lowercases', () => {
    expect(previewMentorSlug('Jane Doe')).toBe('jane-doe');
  });

  it('drops characters outside the slug alphabet', () => {
    expect(previewMentorSlug("O'Brien Jr.!!")).toBe('obrien-jr');
  });

  it('maps a literal dot to a hyphen', () => {
    expect(previewMentorSlug('J. Doe')).toBe('j-doe');
  });

  it('collapses consecutive hyphens and trims leading/trailing ones', () => {
    expect(previewMentorSlug('  -Jane   Doe- ')).toBe('jane-doe');
  });

  it('truncates to 30 characters', () => {
    expect(previewMentorSlug('a'.repeat(40))).toBe('a'.repeat(30));
  });

  // A cut that happens to land right after a hyphen would otherwise leave a
  // trailing hyphen the pattern (and the Go sanitizer, which re-trims after
  // truncating) both reject.
  it('trims a trailing hyphen left by truncation', () => {
    const name = 'a'.repeat(29) + ' ' + 'b'.repeat(10);
    expect(previewMentorSlug(name)).toBe('a'.repeat(29));
  });

  it('returns empty for a name with no latin letters or digits', () => {
    expect(previewMentorSlug('Иван Стрелов')).toBe('');
  });

  it('returns empty for a blank name', () => {
    expect(previewMentorSlug('   ')).toBe('');
  });
});

// This is the fix for a real bug a code review caught: the form binds the live preview
// straight into the field that gets submitted, so leaving "Your URL" untouched sent the
// preview as though the mentor had typed it themselves. SubmitProfile only derives (and
// silently retries on collision) a slug that arrives BLANK — an explicit one is refused
// outright on the first collision — so an untouched preview defeated the whole feature:
// two mentors named "Jane Doe" would give the second one a 409 instead of "jane-doe-2".
describe('resolveMentorSlugForSubmit', () => {
  it('submits nothing when the field was never touched, regardless of the preview', () => {
    expect(resolveMentorSlugForSubmit('jane-doe', false)).toBe('');
  });

  it('submits exactly what the mentor typed once they have touched the field', () => {
    expect(resolveMentorSlugForSubmit('custom-address', true)).toBe('custom-address');
  });

  it('submits empty when the mentor touched the field and then cleared it', () => {
    expect(resolveMentorSlugForSubmit('', true)).toBe('');
  });
});
