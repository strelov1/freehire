import { serverApi } from '$lib/server/api';
import type { CompanyListItem, Job } from '$lib/types';
import type { RequestHandler } from './$types';

// Bounded sitemap built from the existing public listings. A full sitemap-index
// over the whole catalogue (100k+ jobs) plus a lightweight backend slug+updated
// listing is a deliberate follow-up (see design Open Questions); here we cap and
// log what is omitted rather than silently truncating. Cached so crawlers and
// any CDN don't re-run the paging on every hit.
const PAGE = 500;
const JOB_CAP = 5000;
const COMPANY_CAP = 5000;

type Api = ReturnType<typeof serverApi>;

interface Entry {
  loc: string;
  lastmod?: string;
}

// Walk a paginated listing, advancing the offset by the number of items the
// server actually returned — never by the requested page size, which would skip
// rows whenever the backend caps the page lower than asked. Stops on an empty
// page (or `hasMore: false`); flags `capped` if our own cap is reached first.
async function collect<T>(
  cap: number,
  fetchPage: (limit: number, offset: number) => Promise<{ items: T[]; hasMore: boolean }>,
  toEntry: (item: T) => Entry,
): Promise<{ entries: Entry[]; capped: boolean }> {
  const entries: Entry[] = [];
  let offset = 0;
  let capped = false;
  while (offset < cap) {
    const slice = await fetchPage(PAGE, offset);
    if (slice.items.length === 0) break;
    for (const item of slice.items) entries.push(toEntry(item));
    offset += slice.items.length;
    if (!slice.hasMore) break;
    if (offset >= cap) {
      capped = true;
      break;
    }
  }
  return { entries, capped };
}

function escapeXml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

function urlTag({ loc, lastmod }: Entry): string {
  const mod = lastmod ? `\n    <lastmod>${escapeXml(lastmod)}</lastmod>` : '';
  return `  <url>\n    <loc>${escapeXml(loc)}</loc>${mod}\n  </url>`;
}

export const GET: RequestHandler = async ({ url, fetch }) => {
  const origin = url.origin;
  const api = serverApi(fetch);

  const statics: Entry[] = [
    { loc: `${origin}/` },
    { loc: `${origin}/jobs` },
    { loc: `${origin}/companies` },
    { loc: `${origin}/cli` },
  ];

  const [jobs, companies] = await Promise.all([
    collect<Job>(
      JOB_CAP,
      (limit, offset) => api.searchJobs(new URLSearchParams(), limit, offset),
      (job) => ({
        loc: `${origin}/jobs/${job.public_slug}`,
        lastmod: job.updated_at ?? job.posted_at ?? undefined,
      }),
    ),
    collect<CompanyListItem>(
      COMPANY_CAP,
      (limit, offset) => api.listCompanies('', limit, offset),
      (company) => ({ loc: `${origin}/companies/${company.slug}` }),
    ),
  ]);

  if (jobs.capped) {
    console.warn(`sitemap: job URLs capped at ${JOB_CAP}; more exist (full sitemap-index is a follow-up)`);
  }
  if (companies.capped) {
    console.warn(`sitemap: company URLs capped at ${COMPANY_CAP}; more exist`);
  }

  const entries = [...statics, ...jobs.entries, ...companies.entries];
  const body = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${entries.map(urlTag).join('\n')}
</urlset>
`;

  return new Response(body, {
    headers: {
      'content-type': 'application/xml; charset=utf-8',
      'cache-control': 'public, max-age=3600',
    },
  });
};
