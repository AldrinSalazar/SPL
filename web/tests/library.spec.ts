import { test, expect } from '@playwright/test';

test.describe('user sound library', () => {
  test('new sound starts empty, autosaves, renames and removes', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    const nav = page.getByRole('navigation', { name: 'Examples' });

    // (+) creates an empty user sound.
    await page.getByRole('button', { name: 'Create new sound' }).click();
    await expect(page.getByLabel('SPL source')).toHaveValue('');
    await expect(page.getByText('Empty', { exact: true })).toBeVisible();
    await expect(nav.getByRole('button', { name: 'New sound', exact: true })).toBeVisible();

    // Edits autosave to the selected user sound.
    await page.getByLabel('SPL source').fill('spl 2 24000 0.5 0\n');
    await expect.poll(() => page.evaluate(() => localStorage.getItem('spl-studio:sounds:v1') ?? '')).toContain('spl 2 24000');

    // Rename via the title.
    await page.getByRole('button', { name: 'Rename sound New sound' }).click();
    const nameInput = page.getByLabel('Sound name');
    await nameInput.fill('My drone');
    await nameInput.press('Enter');
    await expect(nav.getByRole('button', { name: 'My drone', exact: true })).toBeVisible();
    await expect(page.locator('h1')).toContainText('My drone');

    // Survives reload; selecting restores the saved source.
    await page.reload();
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    await nav.getByRole('button', { name: 'My drone', exact: true }).click();
    await expect(page.getByLabel('SPL source')).toHaveValue('spl 2 24000 0.5 0\n');

    // Remove deletes the chip but keeps the editor text as a draft.
    await page.getByRole('button', { name: 'Remove My drone' }).click();
    await expect(nav.getByRole('button', { name: 'My drone', exact: true })).toHaveCount(0);
    await expect(page.locator('h1')).toContainText('Untitled sound');
    await expect(page.getByLabel('SPL source')).toHaveValue('spl 2 24000 0.5 0\n');
  });

  test('renaming a built-in example forks it into a user sound', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    const nav = page.getByRole('navigation', { name: 'Examples' });

    await nav.getByRole('button', { name: 'Pure tone', exact: true }).click();
    await page.getByRole('button', { name: 'Rename sound Pure tone' }).click();
    const nameInput = page.getByLabel('Sound name');
    await nameInput.fill('Forked tone');
    await nameInput.press('Enter');

    // Built-in stays untouched; a new user chip appears and is selected.
    await expect(nav.getByRole('button', { name: 'Pure tone', exact: true })).toBeVisible();
    await expect(nav.getByRole('button', { name: 'Forked tone', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Remove Forked tone' })).toBeVisible();
    await expect(page.locator('h1')).toContainText('Forked tone');
  });

  test('dropping a .spl file loads it as a new sound', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    const nav = page.getByRole('navigation', { name: 'Examples' });

    await page.evaluate(() => {
      const section = document.querySelector('.source-panel');
      if (!section) throw new Error('source panel missing');
      const dt = new DataTransfer();
      dt.items.add(new File(['spl 2 24000 0.5 0\n'], 'dropped.spl', { type: 'text/plain' }));
      section.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: dt }));
    });

    await expect(page.getByLabel('SPL source')).toHaveValue('spl 2 24000 0.5 0\n');
    await expect(page.locator('h1')).toContainText('dropped');
    await expect(nav.getByRole('button', { name: 'dropped', exact: true })).toBeVisible();
  });
});
