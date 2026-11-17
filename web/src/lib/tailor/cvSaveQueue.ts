// One queue for every whole-document CV save in this tab, shared across workspace instances.
// Saves never overlap: two on the wire can land out of order and leave the older document on
// the server. Sharing it across instances covers a workspace that unmounts with a save still
// queued and is opened again — the remounted one's saves land after the old one's.
let tail: Promise<unknown> = Promise.resolve();

/** Runs `save` once every save queued before it has settled; resolves or rejects with it. */
export function queueCvSave<T>(save: () => Promise<T>): Promise<T> {
  const next = tail.then(save);
  tail = next.catch(() => {});
  return next;
}

/** Settles once every save queued so far has, whatever their outcome. */
export const cvSavesSettled = (): Promise<unknown> => tail;
