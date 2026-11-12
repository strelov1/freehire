/** The identity of the posting a panel conversation belongs to. */
export function pageIdentity(url: string): string {
  try {
    const u = new URL(url);
    const key = `${u.origin}${u.pathname}`;
    if (
      (u.hostname === 'boards.greenhouse.io' || u.hostname === 'job-boards.greenhouse.io') &&
      u.pathname.startsWith('/embed/job_app')
    ) {
      const identity = new URLSearchParams();
      for (const name of ['for', 'token']) {
        const value = u.searchParams.get(name);
        if (value !== null) identity.set(name, value);
      }
      const query = identity.toString();
      return query ? `${key}?${query}` : key;
    }
    return key;
  } catch {
    return url;
  }
}
