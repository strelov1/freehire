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

/** Read the initial page before publishing the account to the panel. */
export async function readStablePageKey(
  read: () => Promise<string>,
  revision: () => number,
  isCurrent: () => boolean,
): Promise<string | null> {
  while (isCurrent()) {
    const before = revision();
    const key = await read();
    if (!isCurrent()) return null;
    if (before === revision()) return key;
  }
  return null;
}
