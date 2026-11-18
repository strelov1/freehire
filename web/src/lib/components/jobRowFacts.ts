// The descriptive lines of a job card, as pure functions of the posting so JobRow keeps only
// the wiring. A card renders either the catalogue's full posting (`Job`) or the tracking
// listing's projection (`JobCard`); each function narrows on what the posting carries, and a
// fact the projection lacks is simply absent.
import { formatSalary, regionLabel, workArrangement } from '#lib/enrichment.js';
import { applyQuestionsLabel, equityLabel } from '#lib/jobFacts.js';
import { metaDescription } from '#lib/seo.js';
import { formatCount } from '#lib/utils.js';
import type { JobRowProps } from './JobRow.props';

type RowJob = JobRowProps['job'];

/**
 * A one-line blurb under the title so a card conveys what the job is without opening it.
 * Prefer the clean model-written summary, but only tech jobs are enriched — fall back to a
 * plain-text snippet of the raw (HTML) posting so non-tech jobs still show a description.
 * metaDescription strips the tags.
 */
export function rowBlurb(job: RowJob): string {
  return 'enrichment' in job
    ? job.enrichment?.summary || metaDescription(job.description ?? '', 220)
    : metaDescription(job.blurb ?? '', 220);
}

/** Salary lives in the enrichment, which a card does not carry: the tracking lists never
 *  showed it (their rows came from the same listing) and a row without one simply omits it. */
export function rowSalary(job: RowJob): string | null {
  return 'enrichment' in job && job.enrichment ? formatSalary(job.enrichment) : null;
}

/** One of two facts only the catalogue's full posting carries, each absent when unknown —
 *  and absent renders nothing: "no equity" and "a long form" are claims we cannot make. */
export function rowEquity(job: RowJob): string | null {
  return 'equity' in job ? equityLabel(job.equity) : null;
}

/** The other: what its form asks. A closed posting takes no applications, so that is
 *  beside the point. */
export function rowQuestions(job: RowJob): string | null {
  return 'apply_questions' in job && !job.closed_at ? applyQuestionsLabel(job.apply_questions) : null;
}

/**
 * How many people have opened the posting — the materialized counter, maintained offline
 * from the access logs (never counted on this read). A card projection does not carry it,
 * so the `in` narrowing is what lets those rows simply omit it, the same way they omit
 * salary. Zero renders nothing at all: "0 views" reads as a dead figure rather than as
 * information.
 */
export function rowViews(job: RowJob): number {
  return 'view_count' in job ? job.view_count : 0;
}

/** The region is dropped when the flags already name the country ("UK · 🇬🇧 United
 *  Kingdom" said one place twice); with no country it is the only geography and stays. */
export function rowRegionTag(job: RowJob): string | null {
  return job.countries?.length ? regionLabel({ regions: job.regions ?? [] }) : null;
}

/** Where the role can be done from — the arrangement ("Remote") and, when no flag names a
 *  country, the region — reads as one thought with the flags and the city, so the three
 *  take a line of their own under the facts instead of being scattered across the end of it. */
export function rowGeoTags(job: RowJob, tags: string[]): string[] {
  return [
    workArrangement({ work_mode: job.work_mode ?? '' }),
    job.countries?.length ? null : regionLabel({ regions: job.regions ?? [] }),
  ].filter((t): t is string => !!t && tags.includes(t));
}

/** The meta line's facts, in order. */
export function rowMetaFacts(o: {
  job: RowJob;
  hideCompany: boolean | undefined;
  posted: string;
  views: number;
  tags: string[];
  regionTag: string | null;
  geoTags: string[];
  equity: string | null;
  questions: string | null;
}): string[] {
  return [
    // With no rail on the employer's own page, its date, views and deadline lead this line.
    ...(o.hideCompany
      ? [
          o.posted,
          o.views > 0 ? `${formatCount(o.views)} ${o.views === 1 ? 'view' : 'views'}` : null,
          o.job.application_deadline ? `Apply by ${o.job.application_deadline}` : null,
        ]
      : []),
    ...o.tags.filter((t) => t !== o.regionTag && !o.geoTags.includes(t)),
    o.equity,
    o.questions,
  ].filter((f): f is string => Boolean(f));
}
