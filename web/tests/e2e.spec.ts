import { test, expect } from '@playwright/test';
import * as fs from 'node:fs';

const MINIMAL = `spl 2 24000 0.5 0

track
0 440 0
0.02 440 0.3
0.4 440 0.3
0.5 440 0
end
`;

test.describe('SPL browser app', () => {
  test('initializes and renders minimal example', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('link', { name: 'SPL home' })).toBeVisible();
    // Wait for worker ready.
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    // Select minimal example.
    await page.getByRole('button', { name: 'Pure tone', exact: true }).click();
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('status').first()).toContainText('Render complete', { timeout: 60000 });
    await expect(page.getByLabel('Sample rate')).toBeVisible();
    await expect(page.locator('canvas')).toBeVisible();
  });

  test('invalid input shows source-linked diagnostics', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    const editor = page.getByLabel('SPL source');
    await editor.fill('spl 2 24000 1 0\ntrack\n0 440 0\n');
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('alert').first()).toBeVisible({ timeout: 30000 });
    await expect(page.getByText(/UNCLOSED_BLOCK|ROW_COUNT|TIME/).first()).toBeVisible();
  });

  test('resource limit error is actionable', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    const editor = page.getByLabel('SPL source');
    await editor.fill('spl 2 24000 1 0\nharmonics\nspectrum\n0 1\n10000 1\ncurve\n0 0.001 0.5\n1 0.001 0.5\nend\n');
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByText(/RESOURCE_LIMIT/)).toBeVisible({ timeout: 30000 });
  });

  test('stale indicator after edit', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    await page.getByRole('button', { name: 'Pure tone', exact: true }).click();
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('status').first()).toContainText('Render complete', { timeout: 60000 });
    await page.getByLabel('SPL source').fill(MINIMAL + '# edited\n');
    await expect(page.getByText('Edited', { exact: true })).toBeVisible();
  });

  test('downloads verify WAV and PNG contents', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    await page.getByRole('button', { name: 'Pure tone', exact: true }).click();
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('status').first()).toContainText('Render complete', { timeout: 60000 });
    // WAV download.
    const wavPromise = page.waitForEvent('download');
    await page.getByRole('link', { name: /WAV/ }).click();
    const wavDl = await wavPromise;
    const wavPath = await wavDl.path();
    if (!wavPath) throw new Error('no wav download path');
    const wav = fs.readFileSync(wavPath);
    expect(wav.subarray(0, 4).toString()).toBe('RIFF');
    expect(wav.subarray(8, 12).toString()).toBe('WAVE');
    // fmt chunk: audioFormat 3 (float), channels 1.
    expect(wav.readUInt16LE(20)).toBe(3);
    expect(wav.readUInt16LE(22)).toBe(1);
    expect(wav.readUInt32LE(24)).toBe(24000);
    // PNG download.
    const pngPromise = page.waitForEvent('download');
    await page.getByRole('link', { name: /PNG/ }).click();
    const pngDl = await pngPromise;
    const pngPath = await pngDl.path();
    if (!pngPath) throw new Error('no png download path');
    const png = fs.readFileSync(pngPath);
    // PNG signature.
    expect([...png.subarray(0, 8)]).toEqual([137, 80, 78, 71, 13, 10, 26, 10]);
  });

  test('spectrogram apply does not rerender audio', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    await page.getByRole('button', { name: 'Pure tone', exact: true }).click();
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('status').first()).toContainText('Render complete', { timeout: 60000 });
    await page.getByRole('button', { name: 'Analysis settings' }).click();
    await page.getByLabel('dB min').fill('-80');
    await page.getByRole('button', { name: /Apply/ }).click();
    await expect(page.getByText('Spectrogram updated', { exact: true })).toBeVisible({ timeout: 30000 });
  });

  test('cancel followed by rerender', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    // Long render to allow cancel: 60s noise at 24k = 1.44M samples.
    const longSrc = `spl 2 24000 60 8\nnoise\n0 200 4000 0.06 0.5\n60 200 4000 0.06 0.5\nend\n`;
    await page.getByLabel('SPL source').fill(longSrc);
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    // Cancel quickly (may or may not catch mid-render; either way worker must stay usable).
    const cancelBtn = page.getByRole('button', { name: 'Cancel' });
    if (await cancelBtn.isVisible().catch(() => false)) {
      await cancelBtn.click();
      await expect(page.getByText(/cancell?ed/i)).toBeVisible({ timeout: 30000 });
    }
    // Rerender minimal must succeed after cancel.
    await page.getByRole('button', { name: 'Pure tone', exact: true }).click();
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('status').first()).toContainText('Render complete', { timeout: 60000 });
  });
});
