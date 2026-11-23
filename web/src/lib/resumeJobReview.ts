import type { components } from '#lib/generated/api.js';
import { SKILL_LABELS } from '#lib/generated/contracts.js';
import { SKILL_ALIASES } from '#lib/generated/skillAliases.js';

type PostingRequirement = components['schemas']['ResumeReviewRequirement'];
type Finding = {
  skill: string;
  kind: 'example' | 'mention' | 'missing';
  snippet?: string;
  requirement?: string;
  priority?: string;
};

const aliases: Readonly<Record<string, readonly string[]>> = SKILL_ALIASES;
const labels: Readonly<Record<string, string>> = SKILL_LABELS;
const action =
  /^(?:I\s+)?(?:automated|built|configured|created|debugged|delivered|deployed|designed|developed|implemented|improved|integrated|led|maintained|managed|migrated|operated|optimized|owned|reduced|shipped|supported|tested|used|wrote)\b/i;

const negation =
  /\b(?:without|never (?:used|worked with)|no (?:hands-on )?experience (?:with|in)|haven['’]t (?:used|worked with)|not familiar with)\s*(?:[^.!?;:\n]*?(?:,|\/|\b(?:and|or)\b)\s*)?$/i;

function escapePattern(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function skillPatterns(skill: string): RegExp[] {
  const words = new Set([labels[skill], ...(aliases[skill] ?? [])].filter((word): word is string => Boolean(word)));
  if (!words.size) words.add(skill.replaceAll('-', ' '));
  const whole = (word: string) => {
    const suffix = skill === 'c' && word === 'C' ? '+#' : '';
    return `(?<![\\p{L}\\p{N}_])${escapePattern(word)}(?![\\p{L}\\p{N}_${suffix}])`;
  };
  return [...words].map((word) => new RegExp(whole(word), /^[a-z]{1,2}$/i.test(word) ? 'u' : 'iu'));
}

function excerpt(line: string, index: number): string {
  const limit = 320;
  if (line.length <= limit) return line;
  const start = Math.max(0, index - 100);
  const end = start + limit;
  return `${start ? '…' : ''}${line.slice(start, end).trim()}${end < line.length ? '…' : ''}`;
}

/** Compare the posting's explicit skills with lines the candidate actually supplied. */
export function reviewResume(
  resume: string,
  skills: readonly string[],
  requirements: readonly PostingRequirement[],
): Finding[] {
  const lines = resume
    .split(/\r?\n/)
    .map((line) => line.trim().replace(/^[•●▪*-]\s+/, ''))
    .filter(Boolean);
  const orderedRequirements = [
    ...requirements.filter(({ priority }) => priority === 'required'),
    ...requirements.filter(({ priority }) => priority !== 'required'),
  ];
  const findings = [...new Set(skills)].map((skill): Finding => {
    const patterns = skillPatterns(skill);
    const requirement = orderedRequirements.find(({ text }) => patterns.some((pattern) => pattern.test(text)));
    const context = requirement ? { requirement: requirement.text, priority: requirement.priority } : {};
    let mention: string | undefined;
    for (const line of lines) {
      const match = patterns
        .map((pattern) => pattern.exec(line))
        .find((candidate) => candidate && !negation.test(line.slice(0, candidate.index)));
      if (!match) continue;
      const snippet = excerpt(line, match.index);
      if (action.test(line)) return { skill, kind: 'example', snippet, ...context };
      mention ??= snippet;
    }
    return mention ? { skill, kind: 'mention', snippet: mention, ...context } : { skill, kind: 'missing', ...context };
  });
  return findings.sort((a, b) => Number(b.priority === 'required') - Number(a.priority === 'required'));
}
