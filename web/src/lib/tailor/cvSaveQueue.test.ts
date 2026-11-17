import { expect, it } from 'vitest';
import { cvSavesSettled, queueCvSave } from './cvSaveQueue.js';

it('starts a save only after the one before it settled, even when that one failed', async () => {
  const order: string[] = [];
  let fail!: (e: Error) => void;
  const first = queueCvSave(() => new Promise<void>((_, reject) => (fail = reject)));
  const second = queueCvSave(async () => {
    order.push('second');
    return 'ok';
  });

  await Promise.resolve();
  expect(order).toEqual([]);

  fail(new Error('down'));
  await expect(first).rejects.toThrow('down');
  await expect(second).resolves.toBe('ok');
  expect(order).toEqual(['second']);
  await expect(cvSavesSettled()).resolves.not.toThrow();
});
