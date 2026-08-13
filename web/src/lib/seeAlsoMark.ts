import { FAMILY_MARKS, type FamilyIconName } from './familymarks';
import { TECH_MARKS } from './techmarks';
import type { SeeAlsoMark } from './collections';

function familyMark(icon: FamilyIconName): SeeAlsoMark {
  return { kind: 'family', icon, color: FAMILY_MARKS[icon].color };
}

// Resolves the one visual mark a "See also" card renders, given the card's
// underlying collection params (undefined for a backer-only collection, which
// carries no facet params) and any brand image a backer lookup already found.
// Precedence: backer image > curated tech logo > country flag > family icon —
// see design.md's "Resolution order in buildSeeAlso()".
export function resolveSeeAlsoMark(
  params: Record<string, string> | null,
  backerImageSrc: string | null
): SeeAlsoMark {
  if (backerImageSrc) return { kind: 'image', src: backerImageSrc };
  if (!params) return familyMark('tech');

  if (params.skills) {
    const tech = TECH_MARKS[params.skills];
    if (tech) return { kind: 'logo', title: tech.title, path: tech.path, hex: tech.hex };
    return familyMark('tech');
  }
  if (params.countries) return { kind: 'flag', countryCode: params.countries };
  if (params.seniority) return familyMark('seniority');
  if (params.category) return familyMark('role');
  if (params.work_mode) return familyMark('remote');

  return familyMark('tech');
}
