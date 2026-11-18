import type { StoredSession } from './session';
import type { StoredMessage } from './wire';

interface RestoreCallbacks {
  recall: () => Promise<StoredSession | null>;
  load: (id: string, token: string) => Promise<StoredMessage[]>;
  canAdopt: () => boolean;
  adopt: (id: string, messages: StoredMessage[]) => void;
  loading: (value: boolean) => void;
  error: (error: unknown) => void | Promise<void>;
}

/** Owns the asynchronous steps of restoring the panel's remembered conversation. */
export function createConversationRestore(callbacks: RestoreCallbacks) {
  let generation = 0;
  // Replacing a restore must not invalidate an active message in the same lifetime.
  let requestId = 0;
  const capture = () => {
    const own = generation;
    return () => own === generation;
  };
  return {
    capture,
    invalidate() {
      generation++;
      callbacks.loading(false);
    },
    async run(token: string | Promise<string | null>, key: string) {
      const request = ++requestId;
      const sameLifetime = capture();
      const current = () => sameLifetime() && request === requestId;
      try {
        const credential = typeof token === 'string' ? token : await token;
        if (!current() || !credential) return;
        const remembered = await callbacks.recall();
        if (!current() || !remembered || remembered.pageKey !== key) return;
        callbacks.loading(true);
        const messages = await callbacks.load(remembered.id, credential);
        if (current() && callbacks.canAdopt()) callbacks.adopt(remembered.id, messages);
      } catch (error) {
        if (current()) await callbacks.error(error);
      } finally {
        if (current()) callbacks.loading(false);
      }
    },
  };
}
