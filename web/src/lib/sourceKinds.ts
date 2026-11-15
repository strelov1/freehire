import { api } from './api';
import type { ProviderKind } from './types';

// Public catalogue data only; all cards share one request, including during navigation.
let pending: Promise<ReadonlyMap<string, ProviderKind>> | undefined;

export function loadSourceKinds(): Promise<ReadonlyMap<string, ProviderKind>> {
  pending ??= Promise.resolve()
    .then(() => api.listSources())
    .then((sources) => new Map(sources.map((source) => [source.source, source.kind])))
    .catch(() => {
      pending = undefined;
      return new Map<string, ProviderKind>();
    });
  return pending;
}
