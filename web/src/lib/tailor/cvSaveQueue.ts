// One queue per CV for its whole-document saves in this tab, shared across workspace instances.
// Saves of one CV never overlap: two on the wire can land out of order and leave the older
// document on the server. Sharing it across instances covers a workspace that unmounts with a
// save still queued and is opened again — the remounted one's saves land after the old one's.
// Keyed by CV, so a save stuck on one CV never holds up another.
const tails = new Map<string, Promise<unknown>>();

/** Runs `save` once every save of `cvId` queued before it has settled; resolves or rejects with it. */
export function queueCvSave<T>(cvId: string, save: () => Promise<T>): Promise<T> {
  const next = (tails.get(cvId) ?? Promise.resolve()).then(save);
  const tail = next.catch(() => {});
  tails.set(cvId, tail);
  void tail.then(() => {
    if (tails.get(cvId) === tail) tails.delete(cvId);
  });
  return next;
}

/** Settles once every save of `cvId` queued so far has, whatever their outcome. */
export const cvSavesSettled = (cvId: string): Promise<unknown> => tails.get(cvId) ?? Promise.resolve();
