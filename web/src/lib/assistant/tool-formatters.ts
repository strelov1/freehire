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

export function classifyFamily(name: string): ToolFamily {
  if (name === 'Bash') return 'bash';
  if (FS_TOOLS.has(name)) return 'fs';
  return 'other';
}

/** Title shown in the collapsed header. */
export function groupTitle(family: ToolFamily, calls: readonly ToolCall[]): string {
  const first = calls[0];
  if (!first) return '';
  if (family === 'bash') {
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
  return readField(input, 'command', 'cmd');
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
  if (family === 'bash') return true;
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
