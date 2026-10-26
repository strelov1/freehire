import type { TechMark } from './techmarks';

let marks = $state<Record<string, TechMark> | null>(null);
let pending = false;

export function ensureTechMarks(): void {
  if (marks || pending) return;
  pending = true;
  void import('./techmarks').then((m) => {
    marks = m.TECH_MARKS;
  });
}

export function techMarks(): Record<string, TechMark> | null {
  return marks;
}
