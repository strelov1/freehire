import type { InsightRoleDemandPoint } from './api-types';

export function groupDemand(points: InsightRoleDemandPoint[]) {
  const groups = new Map<string, InsightRoleDemandPoint[]>();
  for (const point of points) {
    const group = groups.get(point.category);
    if (group) group.push(point);
    else groups.set(point.category, [point]);
  }
  return groups;
}

export function summarizeDemand(counts: number[]) {
  const latest = counts.slice(-4).reduce((sum, count) => sum + count, 0);
  const previous = counts.slice(-8, -4).reduce((sum, count) => sum + count, 0);
  return { latest, previous, change: previous === 0 ? null : (latest - previous) / previous };
}
