/** The employer's people tab, or a company-name search when no usable page is stored. */
export function referralPeopleUrl(companyName: string, linkedInPage?: string): string | null {
  const name = companyName.trim();
  if (!name) return null;

  if (linkedInPage) {
    try {
      const page = new URL(linkedInPage.trim());
      const host = page.hostname.toLowerCase();
      const path = /^\/(company|school)\/([^/]+)(?:\/people)?\/?$/i.exec(page.pathname);
      const kind = path?.[1];
      const slug = path?.[2];
      if (
        (page.protocol === 'https:' || page.protocol === 'http:') &&
        (host === 'linkedin.com' || host.endsWith('.linkedin.com')) &&
        kind &&
        slug
      ) {
        return `https://www.linkedin.com/${kind.toLowerCase()}/${slug}/people/`;
      }
    } catch {
      // A malformed catalogue link has the same fallback as an absent one.
    }
  }

  const search = new URL('https://www.linkedin.com/search/results/people/');
  search.searchParams.set('keywords', name);
  return search.toString();
}
