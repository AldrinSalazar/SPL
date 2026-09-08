import { test, expect } from '@playwright/test';

const tone = (frequency: number) => `spl 2 24000 2 0\ntrack\n0 ${frequency} 0.1\n2 ${frequency} 0.1\nend\n`;

test('initialization failure is visible and render stays disabled', async ({ page }) => {
  await page.route('**/spl.wasm', route => route.fulfill({ status: 404, body: 'missing' }));
  await page.goto('./');
  await expect(page.getByRole('alert')).toContainText('WASM initialization failed');
  await expect(page.getByRole('status')).toHaveText('Worker failed');
  await expect(page.getByRole('button', { name: 'Render', exact: true })).toBeDisabled();
});

test('failed export preserves the previous audio and analysis cache', async ({ page }) => {
  await page.goto('./');
  await expect(page.getByRole('status')).toHaveText('Ready');
  await page.getByLabel('SPL source').fill(tone(440));
  await page.getByRole('button', { name: 'Render', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Render complete');
  const wavURL = await page.getByRole('link', { name: 'WAV', exact: true }).getAttribute('href');
  const imageBytes = () => page.getByRole('link', { name: 'PNG', exact: true }).evaluate(async a => {
    const bytes = new Uint8Array(await (await fetch((a as HTMLAnchorElement).href)).arrayBuffer());
    return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)));
  });
  const original = await imageBytes();
  await page.getByRole('button', { name: 'Analysis settings' }).click();
  await page.getByLabel('SPL source').fill(tone(880));
  await page.getByLabel('dB min', { exact: true }).fill('10');
  await page.getByRole('button', { name: 'Render', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Render failed');
  await expect(page.getByRole('link', { name: 'WAV', exact: true })).toHaveAttribute('href', wavURL!);
  await page.getByLabel('dB min', { exact: true }).fill('-100');
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Spectrogram updated');
  expect(await imageBytes()).toEqual(original);
});

test('integrated player seeks, plays, pauses, stops and fits mobile', async ({ page }, info) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('./');
  await expect(page.getByRole('status')).toHaveText('Ready');
  await page.getByRole('button', { name: 'Render', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Render complete');
  await expect.poll(() => page.locator('canvas').evaluate(c => c.width)).toBe(1000);
  await page.screenshot({ path: info.outputPath('desktop.png'), fullPage: true });
  const bounds = await page.locator('canvas').boundingBox();
  await page.locator('canvas').click({ position: { x: bounds!.width * 0.4, y: bounds!.height / 2 } });
  await expect.poll(async () => Number(await page.getByRole('slider', { name: 'seek seconds' }).inputValue())).toBeCloseTo(0.8, 2);
  await page.getByRole('button', { name: 'Play', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
  await expect.poll(async () => Number(await page.getByRole('slider', { name: 'seek seconds' }).inputValue())).toBeGreaterThan(0.85);
  await page.getByRole('button', { name: 'Pause', exact: true }).click();
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  await expect(page.getByRole('slider', { name: 'seek seconds' })).toHaveValue('0');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: info.outputPath('mobile.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
