import { describe, expect, it } from 'vitest';
import { createConversationRestore } from './conversationRestore';
import type { StoredSession } from './session';
import type { StoredMessage } from './wire';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

const transcript: StoredMessage[] = [{ seq: 1, role: 'user', content: { text: 'private chat' } }];

function harness() {
  let adopted: StoredMessage[] = [];
  let loading = false;
  let error: unknown;
  let occupied = false;
  const forgotten: string[] = [];
  const remembered = deferred<StoredSession | null>();
  const response = deferred<StoredMessage[]>();
  const restore = createConversationRestore({
    recall: () => remembered.promise,
    load: () => response.promise,
    canAdopt: () => !occupied,
    adopt: (_id, messages) => {
      adopted = messages;
    },
    loading: (value) => {
      loading = value;
    },
    error: (err) => {
      error = err;
      forgotten.push('missing session');
    },
  });
  return {
    restore,
    remembered,
    response,
    occupy: () => {
      occupied = true;
    },
    state: () => ({ adopted, loading, error, forgotten }),
  };
}

describe('conversation restoration', () => {
  it('restores the remembered chat after delayed credentials resolve in the same lifetime', async () => {
    const token = deferred<string | null>();
    let adopted: StoredMessage[] = [];
    const restore = createConversationRestore({
      recall: async () => ({ id: 'chat-a', pageKey: 'page-a' }),
      load: async (_id, credential) => (credential === 'token-a' ? transcript : []),
      canAdopt: () => true,
      adopt: (_id, messages) => {
        adopted = messages;
      },
      loading: () => {},
      error: () => {},
    });
    const pending = restore.run(token.promise, 'page-a');
    token.resolve('token-a');
    await pending;
    expect(adopted).toEqual(transcript);
  });

  it('does not restore after delayed credentials outlive a page or account change', async () => {
    const h = harness();
    const token = deferred<string | null>();
    const pending = h.restore.run(token.promise, 'page-a');
    h.restore.invalidate();
    token.resolve('token-a');
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    h.response.resolve(transcript);
    await pending;
    expect(h.state().adopted).toEqual([]);
  });

  it('adopts a valid conversation for the current page', async () => {
    const h = harness();
    const pending = h.restore.run('token-a', 'page-a');
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    h.response.resolve(transcript);
    await pending;
    expect(h.state().adopted).toEqual(transcript);
    expect(h.state().loading).toBe(false);
  });

  it('discards a transcript after the conversation lifetime is invalidated', async () => {
    const h = harness();
    const pending = h.restore.run('token-a', 'page-a');
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    await Promise.resolve();
    expect(h.state().loading).toBe(true);
    h.restore.invalidate();
    h.response.resolve(transcript);
    await pending;
    expect(h.state().adopted).toEqual([]);
    expect(h.state().loading).toBe(false);
  });

  it('invalidates restoration before the stored session read resolves', async () => {
    const h = harness();
    const pending = h.restore.run('token-a', 'page-a');
    h.restore.invalidate();
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    h.response.resolve(transcript);
    await pending;
    expect(h.state().adopted).toEqual([]);
  });

  it('does not overwrite a conversation started while the transcript was loading', async () => {
    const h = harness();
    const pending = h.restore.run('token-a', 'page-a');
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    h.occupy();
    h.response.resolve(transcript);
    await pending;
    expect(h.state().adopted).toEqual([]);
  });

  it('does not invalidate active conversation work when restoration starts', async () => {
    const h = harness();
    const token = deferred<string | null>();
    h.occupy();
    const current = h.restore.capture();
    const pending = h.restore.run(token.promise, 'page-a');
    token.resolve('token-a');
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    h.response.resolve(transcript);
    await pending;
    expect(current()).toBe(true);
    expect(h.state().adopted).toEqual([]);
  });

  it('discards stale errors instead of forgetting a new account session', async () => {
    const h = harness();
    const pending = h.restore.run('token-a', 'page-a');
    h.remembered.resolve({ id: 'chat-a', pageKey: 'page-a' });
    await Promise.resolve();
    h.restore.invalidate();
    h.response.reject(new Error('old session missing'));
    await pending;
    expect(h.state().forgotten).toEqual([]);
    expect(h.state().error).toBeUndefined();
  });

  it('does not let an older completion clear a newer restore loading state', async () => {
    let loading = false;
    const old = deferred<StoredMessage[]>();
    const fresh = deferred<StoredMessage[]>();
    const adopted: string[] = [];
    const restore = createConversationRestore({
      recall: async () => ({ id: 'remembered', pageKey: 'page-a' }),
      load: (_id, token) => (token === 'old' ? old.promise : fresh.promise),
      canAdopt: () => true,
      adopt: (_id, messages) => {
        adopted.push(String(messages[0]?.content));
      },
      loading: (value) => {
        loading = value;
      },
      error: () => {},
    });
    const first = restore.run('old', 'page-a');
    await Promise.resolve();
    const second = restore.run('fresh', 'page-a');
    await Promise.resolve();
    old.resolve([]);
    await first;
    expect(loading).toBe(true);
    expect(adopted).toEqual([]);
    fresh.resolve([{ seq: 1, role: 'user', content: 'fresh chat' }]);
    await second;
    expect(loading).toBe(false);
    expect(adopted).toEqual(['fresh chat']);
  });
});
