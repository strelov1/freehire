import { expect, it } from 'vitest';
import { cvSavesSettled, queueCvSave } from './cvSaveQueue.js';

it('starts a save only after the one before it settled, even when that one failed', async () => {
  const order: string[] = [];
  let fail!: (e: Error) => void;
  const first = queueCvSave('cv-a', () => new Promise<void>((_, reject) => (fail = reject)));
  const second = queueCvSave('cv-a', async () => {
    order.push('second');
    return 'ok';
  });

  await Promise.resolve();
  expect(order).toEqual([]);

  fail(new Error('down'));
  await expect(first).rejects.toThrow('down');
  await expect(second).resolves.toBe('ok');
  expect(order).toEqual(['second']);
  await expect(cvSavesSettled('cv-a')).resolves.not.toThrow();
});

it('does not hold one CV behind a save of another that is still on the wire', async () => {
  let finishA!: () => void;
  const a = queueCvSave('cv-a', () => new Promise<void>((resolve) => (finishA = resolve)));

  await expect(queueCvSave('cv-b', async () => 'saved')).resolves.toBe('saved');
  await expect(cvSavesSettled('cv-b')).resolves.not.toThrow();

  finishA();
  await a;
});
