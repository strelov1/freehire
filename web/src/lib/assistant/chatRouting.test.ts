import { describe, expect, it } from 'vitest';
import { addressStep, autostartTurn } from './chatRouting';

describe('autostartTurn', () => {
  const base = { kickoff: undefined, empty: true, preset: 'chat', autoRun: false };

  it('starts nothing on a conversation that already has messages', () => {
    expect(autostartTurn({ ...base, empty: false, kickoff: 'hi', autoRun: true })).toBeNull();
  });

  it('sends the kickoff first, then a rehearsal opening, then an unattended run', () => {
    expect(autostartTurn({ ...base, kickoff: 'hi', preset: 'interview', autoRun: true })).toEqual({
      kind: 'message',
      text: 'hi',
    });
    expect(autostartTurn({ ...base, preset: 'interview', autoRun: true })).toEqual({ kind: 'opening' });
    expect(autostartTurn({ ...base, autoRun: true })).toEqual({ kind: 'autopilot' });
    expect(autostartTurn(base)).toBeNull();
  });
});

describe('addressStep', () => {
  const pane = { navigatingTo: null, deferredNew: false, activeId: 'a' };

  it('waits while a navigation the chat started is in flight, and clears the guard when it lands', () => {
    expect(addressStep('a', { ...pane, navigatingTo: 'b' })).toEqual({ clearGuard: false, action: 'none' });
    expect(addressStep('b', { ...pane, navigatingTo: 'b' })).toEqual({ clearGuard: true, action: 'open' });
  });

  it('boots again on the bare address, except over a blank chat', () => {
    expect(addressStep(undefined, pane)).toEqual({ clearGuard: false, action: 'boot' });
    expect(addressStep(undefined, { ...pane, deferredNew: true, activeId: null })).toEqual({
      clearGuard: false,
      action: 'none',
    });
  });

  it('opens a different conversation and ignores the one already open', () => {
    expect(addressStep('b', pane)).toEqual({ clearGuard: false, action: 'open' });
    expect(addressStep('a', pane)).toEqual({ clearGuard: false, action: 'none' });
  });
});
