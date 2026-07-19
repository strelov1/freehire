import { describe, it, expect } from 'vitest';
import {
  classifyFamily,
  freehireLabel,
  isFreehireGroup,
  commandLine,
  groupTitle,
  isExpandable,
  bashCommand,
} from './tool-formatters';

describe('tool-formatters — freehire intent labels', () => {
  it('groups the Terminal tool with bash', () => {
    expect(classifyFamily('Terminal')).toBe('bash');
    expect(classifyFamily('Bash')).toBe('bash');
    expect(classifyFamily('Read')).toBe('fs');
  });

  it('maps freehire subcommands to friendly labels (longest path wins)', () => {
    expect(freehireLabel('freehire cv context 12')).toBe('Reading the fit analysis');
    expect(freehireLabel('freehire cv get 12')).toBe('Reading your CV');
    expect(freehireLabel('freehire cv edit 12 --patch x')).toBe('Updating your CV');
    expect(freehireLabel('freehire search golang --json')).toBe('Searching jobs');
    expect(freehireLabel('freehire whatever-new-cmd')).toBe('Working with freehire');
    expect(freehireLabel('printenv')).toBeNull();
    expect(freehireLabel('ls -la')).toBeNull();
  });

  it('reads the command from command/cmd, an argv array, or a bare string', () => {
    expect(bashCommand({ command: 'freehire cv get 1' })).toBe('freehire cv get 1');
    expect(bashCommand({ args: ['freehire', 'cv', 'get', '1'] })).toBe('freehire cv get 1');
    expect(bashCommand('freehire facets')).toBe('freehire facets');
    expect(bashCommand({})).toBeNull();
  });

  it('titles a freehire group with distinct intent labels, not the raw command', () => {
    const calls = [
      { name: 'Terminal', input: { command: 'freehire cv context 12' } },
      { name: 'Terminal', input: { command: 'freehire cv get 12' } },
    ];
    expect(isFreehireGroup(calls)).toBe(true);
    const title = groupTitle('bash', calls);
    expect(title).toBe('Reading the fit analysis · Reading your CV');
    expect(title).not.toContain('freehire');
    expect(title).not.toContain('$');
  });

  it('caps a long freehire group title', () => {
    const calls = [
      { name: 'Terminal', input: { command: 'freehire cv context 1' } },
      { name: 'Terminal', input: { command: 'freehire cv get 1' } },
      { name: 'Terminal', input: { command: 'freehire search go' } },
    ];
    expect(groupTitle('bash', calls)).toBe('Reading the fit analysis · Reading your CV · +1');
  });

  it('a single freehire call is a flat chip (not expandable) and hides the command', () => {
    const calls = [{ name: 'Terminal', input: { command: 'freehire cv get 12' } }];
    expect(groupTitle('bash', calls)).toBe('Reading your CV');
    expect(isExpandable('bash', calls)).toBe(false);
  });

  it('non-freehire shell commands keep the raw shell rendering', () => {
    const calls = [{ name: 'Bash', input: { command: 'ls -la' } }];
    expect(isFreehireGroup(calls)).toBe(false);
    expect(groupTitle('bash', calls)).toBe('Ran ls -la');
    expect(commandLine({ command: 'ls -la' })).toBe('$ ls -la');
    expect(commandLine({ command: 'freehire cv get 1' })).toBe('Reading your CV');
  });
});
