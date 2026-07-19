// Per-tool header/line formatting for the assistant chat transcript. Centralizes
// the claude-style "Ran <cmd>" / "Explored N files" affordances so the renderer
// in the assistant page stays free of name-dispatch ladders.
//
// Ported from roy-web (`workspace/src/lib/tool-formatters.ts`); the only local
// change is inlining `truncate` (roy imports it from `./utils`).

export type ToolFamily = 'fs' | 'bash' | 'other';

export interface ToolCall {
  name: string;
  input: unknown;
}

/** Truncate `s` to at most `n` characters, appending an ellipsis when cut. */
function truncate(s: string, n: number): string {
  return s.length <= n ? s : s.slice(0, n - 1) + '…';
}

const FS_TOOLS = new Set(['Read', 'Glob', 'Grep', 'LS']);

/** The shell command a call carries. roy surfaces a `freehire` invocation two
 *  ways: as `{name:'Terminal', input:{command}}` (an explicit shell tool) or —
 *  when the ACP tool title IS the command — as `{name:'<command>', input:{…}}`.
 *  Read it from the input, falling back to the name so the command resolves in
 *  both shapes. */
function callCommand(call: ToolCall): string | null {
  return bashCommand(call.input) ?? (call.name || null);
}

export function classifyFamily(call: ToolCall): ToolFamily {
  const { name } = call;
  // The assistant runs the `freehire` CLI through the harness shell tool, which
  // the ACP layer surfaces as `Terminal` (Claude Code) — group it with `Bash`.
  if (name === 'Bash' || name === 'Terminal') return 'bash';
  if (FS_TOOLS.has(name)) return 'fs';
  // A `freehire` call whose ACP title (name) is the command string itself would
  // otherwise fall through to `other` and render as `Called <command>` + a JSON
  // dump. Recognize it by its command and group it with bash so it reads as an
  // intent label.
  if (labelFor(call) !== null) return 'bash';
  return 'other';
}

/** A shell tool call carrying no command. roy relays a pending ACP tool
 *  notification (title `Bash`, no `raw_input`) before the settled one, which
 *  unfiltered renders as an empty "Ran command". The renderer drops these before
 *  grouping so only the real, command-bearing call shows. */
export function isNoiseShellCall(call: ToolCall): boolean {
  return (call.name === 'Bash' || call.name === 'Terminal') && bashCommand(call.input) === null;
}

// Map a `freehire <subcommand>` invocation to a neutral, human-readable label so
// the transcript reads as intent ("Reading your CV") rather than a raw shell line
// — the assistant's only shell tool is the freehire CLI. Longest sub-command path
// wins (e.g. `cv context` before `cv`).
const FREEHIRE_LABELS: Record<string, string> = {
  'cv context': 'Reading the fit analysis',
  'cv get': 'Reading your CV',
  'cv edit': 'Updating your CV',
  'cv render': 'Rendering your CV',
  search: 'Searching jobs',
  job: 'Reading a job posting',
  company: 'Reading a company',
  facets: 'Loading filters',
  'market-fit': 'Analyzing market fit',
};

/** Friendly label for a `freehire …` command, or `null` if it is not one. */
export function freehireLabel(command: string): string | null {
  const parts = command.trim().split(/\s+/);
  if (parts[0] !== 'freehire') return null;
  const two = parts.slice(1, 3).join(' ');
  const one = parts[1] ?? '';
  return FREEHIRE_LABELS[two] ?? FREEHIRE_LABELS[one] ?? 'Working with freehire';
}

/** Intent label for a call's shell command, or `null` when it is not a freehire
 *  call (no command, or a non-freehire program). */
function labelFor(call: ToolCall): string | null {
  const cmd = callCommand(call);
  return cmd ? freehireLabel(cmd) : null;
}

/** True when every call in the group is a `freehire` command — render as intent
 *  labels with no shell chrome, and never surface the raw command. */
export function isFreehireGroup(calls: readonly ToolCall[]): boolean {
  return calls.length > 0 && calls.every((c) => labelFor(c) !== null);
}

/** One expanded line for a shell/terminal call: the friendly `freehire` label, or
 *  `$ <command>` for any other command. */
export function commandLine(call: ToolCall): string {
  const cmd = callCommand(call);
  if (!cmd) return 'Command';
  return freehireLabel(cmd) ?? `$ ${cmd}`;
}

/** Title shown in the collapsed header. */
export function groupTitle(family: ToolFamily, calls: readonly ToolCall[]): string {
  const first = calls[0];
  if (!first) return '';
  if (family === 'bash') {
    if (isFreehireGroup(calls)) {
      // Collapse to the distinct intent labels ("Reading the fit analysis ·
      // Reading your CV"), capped so the header stays short.
      const distinct = [
        ...new Set(calls.map((c) => labelFor(c)).filter((l): l is string => l !== null)),
      ];
      if (distinct.length <= 2) return distinct.join(' · ');
      return `${distinct.slice(0, 2).join(' · ')} · +${distinct.length - 2}`;
    }
    if (calls.length === 1) {
      const cmd = bashCommand(first.input);
      return cmd ? `Ran ${shortenCommand(cmd)}` : 'Ran command';
    }
    return `Ran ${calls.length} commands`;
  }
  if (family === 'fs') {
    if (calls.length === 1) return callLine(first);
    return `Explored ${calls.length} files`;
  }
  const name = first.name;
  return calls.length > 1 ? `Called ${name} × ${calls.length}` : `Called ${name}`;
}

/** One line in the expanded list. */
export function callLine(call: ToolCall): string {
  switch (call.name) {
    case 'Read': {
      const p = readField(call.input, 'file_path', 'path');
      return p ? `Read ${basename(p)}` : 'Read';
    }
    case 'Glob': {
      const p = readField(call.input, 'pattern');
      return p ? `Globbed ${p}` : 'Glob';
    }
    case 'Grep': {
      const p = readField(call.input, 'pattern');
      return p ? `Grepped ${p}` : 'Grep';
    }
    case 'LS': {
      const p = readField(call.input, 'path');
      return p ? `Listed ${basename(p)}` : 'LS';
    }
    case 'Bash': {
      const cmd = bashCommand(call.input);
      return cmd ? `$ ${cmd}` : 'Bash';
    }
    case 'Write':
    case 'Edit': {
      const p = readField(call.input, 'file_path', 'path');
      return p ? `${call.name} ${basename(p)}` : call.name;
    }
    default:
      return call.name;
  }
}

export function bashCommand(input: unknown): string | null {
  if (typeof input === 'string') return input.length > 0 ? input : null;
  const cmd = readField(input, 'command', 'cmd');
  if (cmd) return cmd;
  // ACP terminal calls may carry argv instead of a single command string.
  if (input && typeof input === 'object') {
    const args = (input as Record<string, unknown>).args;
    if (Array.isArray(args)) {
      const parts = args.filter((a): a is string => typeof a === 'string');
      if (parts.length > 0) return parts.join(' ');
    }
  }
  return null;
}

export function nonEmptyInput(input: unknown): boolean {
  if (input === null || input === undefined) return false;
  if (typeof input === 'object' && Object.keys(input as object).length === 0) return false;
  return true;
}

export function previewToolInput(input: unknown): string {
  try {
    return truncate(JSON.stringify(input), 200);
  } catch {
    return String(input);
  }
}

/** Whether the group's body adds anything beyond its title — used to decide
 *  between a flat chip and an expandable `<details>` in the renderer. */
export function isExpandable(family: ToolFamily, calls: readonly ToolCall[]): boolean {
  if (family === 'bash') {
    // A single freehire call is fully described by its intent label — flat chip.
    if (isFreehireGroup(calls) && calls.length === 1) return false;
    return true;
  }
  if (family === 'fs') return calls.length > 1;
  return calls.some((c) => nonEmptyInput(c.input));
}

function readField(input: unknown, ...keys: string[]): string | null {
  if (!input || typeof input !== 'object') return null;
  const obj = input as Record<string, unknown>;
  for (const k of keys) {
    const v = obj[k];
    if (typeof v === 'string' && v.length > 0) return v;
  }
  return null;
}

function basename(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
  return i >= 0 ? path.slice(i + 1) : path;
}

const TITLE_MAX = 60;
function shortenCommand(cmd: string): string {
  const nl = cmd.indexOf('\n');
  const firstLine = (nl >= 0 ? cmd.slice(0, nl) : cmd).trim();
  return truncate(firstLine, TITLE_MAX);
}
