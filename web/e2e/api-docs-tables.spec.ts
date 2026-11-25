import { expect, test } from '@playwright/test';

// A markdown table in the API reference's overview must scroll inside its column, not
// run under the request pane beside it: the API plans tier table is seven columns wide.
test('API reference tables stay inside their column', async ({ page }) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/docs/api');
  const table = page.locator('.markdown table').filter({ hasText: 'Requests per 60 s' });
  await expect(table).toBeVisible({ timeout: 30_000 });
  const box = await table.evaluate((t) => ({
    right: t.getBoundingClientRect().right,
    column: (t.parentElement ?? t).getBoundingClientRect().right,
    overflows: t.scrollWidth > t.clientWidth,
  }));
  expect(box.overflows).toBe(true);
  expect(box.right).toBeLessThanOrEqual(box.column + 1);
});
