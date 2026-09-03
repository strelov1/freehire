// Brand marks for the applicant-tracking systems whose application forms we
// capture, keyed by the provider string the captured form carries. Mirrors
// techmarks.ts — a flat, hand-maintained map to a verified source rather than an
// auto-derived guess — and feeds the same `BrandMark` primitive.
//
// ONE entry, and that is the whole state of the art: `simple-icons` carries no
// mark for Ashby, Workable, Lever or Recruitee. (`unilever` is a slug collision,
// not Lever.) That is why the job page shows the mark BESIDE the provider's name
// instead of in place of it — a block identified by its mark alone would name its
// source on Greenhouse postings and leave it unnamed on the other four in five.
//
// `siGreenhouse` was checked to be the ATS rather than a same-named brand: its
// `source` is https://brand.greenhouse.io/brand-portal/p/6. techmarks.ts records
// three cases where an exact slug match resolved to the wrong identity (`elk`,
// `hive`, `backbone`), so the source check is the bar here, not extra caution.
// Anything added below gets the same check.
//
// simple-icons also REMOVES marks on a trademark request — it has already dropped
// AWS, Java and C#. If Greenhouse ever goes the same way, the lookup returns
// undefined and the caption degrades to text, which is the path four providers
// already take. Nothing else needs to change.
import { siGreenhouse } from 'simple-icons';

export type AtsMark = {
  title: string;
  path: string;
  hex: string;
};

export const ATS_MARKS: Record<string, AtsMark> = {
  greenhouse: siGreenhouse,
};
