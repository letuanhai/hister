// SPDX-License-Identifier: AGPL-3.0-or-later
import { expect, test } from '@playwright/test';

import { mockAPI, original, preview, previewPath } from './fixtures';

test.beforeEach(async ({ page }) => {
  await mockAPI(page);
});

test('a previewed page can be saved and read from the saved pages', async ({ page }) => {
  await page.goto(previewPath());
  await page.getByRole('button', { name: 'Save for offline', exact: true }).click();
  await expect(page.locator('.preview-offline-status')).toHaveText('Saved for offline reading.');
  await expect(page.getByRole('button', { name: 'Remove from offline reading' })).toBeVisible();

  await page.goto('/saved');
  const entry = page.getByRole('button', { name: new RegExp(preview.title) });
  await expect(entry).toBeVisible();
  await expect(page.getByText('stored preview')).toBeVisible();
  await page.getByLabel('Filter saved pages').fill('nothing matches');
  await expect(page.getByText('No saved page matches the filter.')).toBeVisible();
  await page.getByLabel('Filter saved pages').fill('');
  await entry.click();
  await expect(page.frameLocator('iframe').getByText('Saved article content')).toBeVisible();
  await expect(page.locator('iframe')).toHaveAttribute('sandbox', /^((?!allow-scripts).)*$/);
  await page.getByRole('button', { name: 'All saved pages' }).click();
  await page.getByRole('button', { name: `Remove ${preview.title}` }).click();
  await expect(page.getByText('No pages saved yet.')).toBeVisible();
  expect(original).toBeTruthy();
});
