import { test, expect } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execSync } from 'node:child_process';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

function readWavFloat32(p: string): { rate: number; samples: Float32Array } {
  const b = fs.readFileSync(p);
  // Minimal RIFF parse: find 'data' chunk.
  let off = 12;
  let rate = 0;
  let samples = new Float32Array(0);
  while (off + 8 <= b.length) {
    const id = b.subarray(off, off + 4).toString();
    const size = b.readUInt32LE(off + 4);
    const start = off + 8;
    if (id === 'fmt ') {
      rate = b.readUInt32LE(start + 4);
    } else if (id === 'data') {
      const count = Math.floor(size / 4);
      samples = new Float32Array(count);
      for (let i = 0; i < count; i++) samples[i] = b.readFloatLE(start + i * 4);
    }
    off = start + size + (size % 2);
  }
  return { rate, samples };
}

const PARITY_CASES = [
  { example: 'minimal', file: 'minimal-track.spl' },
  { example: 'metallic', file: 'metallic-impact.spl' },
  { example: 'two-noises', file: 'two-noises.spl' }
];

for (const c of PARITY_CASES) {
  test(`native vs browser parity: ${c.file}`, async ({ page }) => {
    const tmpDir = test.info().outputDir;
    const nativeWav = path.join(tmpDir, `native-${c.file}.wav`);
    const nativePng = path.join(tmpDir, `native-${c.file}.png`);
    const webDir = path.join(__dirname, '..');
    const repoRoot = path.join(webDir, '..');
    execSync(`go run ./cmd/spl render examples/${c.file} --wav "${nativeWav}" --spectrogram "${nativePng}" --force`, {
      cwd: repoRoot, stdio: 'pipe'
    });

    await page.goto('./');
    await expect(page.getByRole('status').first()).toContainText(/ready|render/i, { timeout: 60000 });
    await page.getByRole('button', { name: ({ minimal: 'Pure tone', metallic: 'Metallic impact', 'two-noises': 'Moving noise' } as Record<string, string>)[c.example], exact: true }).click();
    await page.getByRole('button', { name: 'Render', exact: true }).click();
    await expect(page.getByRole('status').first()).toContainText('Render complete', { timeout: 60000 });

    const wavPromise = page.waitForEvent('download');
    await page.getByRole('link', { name: /WAV/ }).click();
    const wavDl = await wavPromise;
    const browserWav = path.join(tmpDir, `browser-${c.file}.wav`);
    await wavDl.saveAs(browserWav);

    const pngPromise = page.waitForEvent('download');
    await page.getByRole('link', { name: /PNG/ }).click();
    const pngDl = await pngPromise;
    const browserPng = path.join(tmpDir, `browser-${c.file}.png`);
    await pngDl.saveAs(browserPng);

    const n = readWavFloat32(nativeWav);
    const br = readWavFloat32(browserWav);
    expect(br.rate).toBe(n.rate);
    expect(br.samples.length).toBe(n.samples.length);
    let maxAbs = 0;
    let maxRel = 0;
    for (let i = 0; i < n.samples.length; i++) {
      const a = n.samples[i];
      const bval = br.samples[i];
      const ad = Math.abs(a - bval);
      if (ad > maxAbs) maxAbs = ad;
      const denom = Math.max(1, Math.abs(a), Math.abs(bval));
      const rd = ad / denom;
      if (rd > maxRel) maxRel = rd;
    }
    console.log(`parity ${c.file}: samples=${n.samples.length} maxAbs=${maxAbs} maxRel=${maxRel}`);
    expect(maxAbs).toBeLessThan(1e-5);
    expect(maxRel).toBeLessThan(1e-5);

    const nb = fs.readFileSync(nativePng);
    const bb = fs.readFileSync(browserPng);
    console.log(`png ${c.file}: native=${nb.length} browser=${bb.length} identical=${nb.equals(bb)}`);
    expect(bb.subarray(1, 4).toString()).toBe('PNG');
    expect(Math.abs(nb.length - bb.length) / nb.length).toBeLessThan(0.05);
  });
}
