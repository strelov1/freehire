// Mirrors internal/identity/username.Sanitize plus the word-join
// internal/engage/mentorship's SubmitProfile applies before calling it — see profile.go.
// This is a PREVIEW only: the backend is the sole authority on the slug actually stored,
// so this never needs the fallback-to-"user" behavior the backend has for a name that
// sanitizes to nothing — an empty preview here just means "generated automatically".
const MAX_LEN = 30;

export function previewMentorSlug(name: string): string {
  const joined = name.trim().split(/\s+/).filter(Boolean).join('-');
  let out = '';
  for (const ch of joined.toLowerCase()) {
    if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch === '-') {
      out += ch;
    } else if (ch === '.') {
      out += '-';
    }
  }
  out = out.replace(/-+/g, '-').replace(/^-+|-+$/g, '');
  // Truncating can land right after a hyphen — re-trim, mirroring the Go sanitizer's
  // own post-truncate trim.
  return out.slice(0, MAX_LEN).replace(/-+$/, '');
}

// The form shows `previewMentorSlug`'s result live in the URL field so a mentor sees a
// plausible address as they type their name — but that preview must NOT be what gets
// submitted unless the mentor deliberately edited the field. SubmitProfile derives (and
// silently retries on collision) a slug ONLY when it arrives blank; an explicit one is
// refused outright on the first collision, with no retry — see profile.go's
// SubmitProfile. Sending the untouched preview as if it were typed would defeat that:
// two mentors named "Jane Doe" would give the second a hard 409 instead of the promised
// "jane-doe-2".
export function resolveMentorSlugForSubmit(slug: string, touched: boolean): string {
  return touched ? slug : '';
}
