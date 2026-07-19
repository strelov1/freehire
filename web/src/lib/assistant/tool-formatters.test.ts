import { describe, it, expect } from 'vitest';
import {
  classifyFamily,
  freehireLabel,
  isFreehireGroup,
  commandLine,
  groupTitle,
  isExpandable,
  isNoiseShellCall,
  bashCommand,
} from './tool-formatters';

describe('tool-formatters — freehire intent labels', () => {
  it('groups the Terminal tool with bash', () => {
    expect(classifyFamily({ name: 'Terminal', input: {} })).toBe('bash');
    expect(classifyFamily({ name: 'Bash', input: {} })).toBe('bash');
    expect(classifyFamily({ name: 'Read', input: {} })).toBe('fs');
    expect(classifyFamily({ name: 'SomeTool', input: {} })).toBe('other');
  });

  it('classifies a freehire call as bash even when the tool name IS the command', () => {
    // roy surfaces a Bash call with the ACP title = the command string, so `name`
    // is the command itself ("freehire cv context 14"), not "Bash"/"Terminal".
    const call = {
      name: 'freehire cv context 14',
      input: { command: 'freehire cv context 14', description: 'Load fit analysis' },
    };
    expect(classifyFamily(call)).toBe('bash');
    expect(isFreehireGroup([call])).toBe(true);
    // Flat intent label, no raw command, no JSON dump.
    expect(groupTitle('bash', [call])).toBe('Reading the fit analysis');
    expect(isExpandable('bash', [call])).toBe(false);
  });

  it('drops the empty pending shell frame that precedes a completed call', () => {
    // ACP emits a pending Bash notification (no command yet) before the settled
    // one; unfiltered it renders as a bare, empty "Ran command".
    expect(isNoiseShellCall({ name: 'Bash', input: {} })).toBe(true);
    expect(isNoiseShellCall({ name: 'Terminal', input: { description: 'x' } })).toBe(true);
    expect(isNoiseShellCall({ name: 'Terminal', input: { command: 'freehire facets' } })).toBe(false);
    expect(isNoiseShellCall({ name: 'freehire cv get 1', input: { command: 'freehire cv get 1' } })).toBe(false);
    expect(isNoiseShellCall({ name: 'Read', input: {} })).toBe(false);
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
    expect(commandLine({ name: 'Bash', input: { command: 'ls -la' } })).toBe('$ ls -la');
    expect(commandLine({ name: 'Terminal', input: { command: 'freehire cv get 1' } })).toBe(
      'Reading your CV',
    );
  });
});
