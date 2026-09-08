// Builds Go WASM engine and copies the matching runtime support JavaScript.
// Resolves wasm_exec.js from the exact toolchain (go env GOROOT) instead of a
// hardcoded legacy path. Run via `npm run build` / `npm run dev` (pre hooks).
import { execSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const webDir = join(here, '..');
const publicDir = join(webDir, 'public');
mkdirSync(publicDir, { recursive: true });

function sh(cmd, opts = {}) {
  return execSync(cmd, { encoding: 'utf8', ...opts }).trim();
}

const goVersion = sh('go version');
console.log(`[wasm] ${goVersion}`);
const goroot = sh('go env GOROOT');
const candidates = [
  join(goroot, 'lib', 'wasm', 'wasm_exec.js'),
  join(goroot, 'misc', 'wasm', 'wasm_exec.js')
];
let src = null;
for (const c of candidates) {
  if (existsSync(c)) { src = c; break; }
}
if (!src) {
  console.error(`[wasm] wasm_exec.js not found under ${goroot} (tried ${candidates.join(', ')})`);
  process.exit(1);
}
console.log(`[wasm] runtime JS: ${src}`);
copyFileSync(src, join(publicDir, 'wasm_exec.js'));

// Build WASM from repo root (web/..).
const repoRoot = join(webDir, '..');
execSync('go build -o web/public/spl.wasm ./cmd/splwasm', {
  cwd: repoRoot,
  stdio: 'inherit',
  env: { ...process.env, GOOS: 'js', GOARCH: 'wasm' }
});

const wasmStat = statSync(join(publicDir, 'spl.wasm'));
const jsStat = statSync(join(publicDir, 'wasm_exec.js'));
const info = {
  goVersion,
  goroot,
  jsSource: src,
  builtAt: new Date().toISOString(),
  wasmBytes: wasmStat.size,
  jsBytes: jsStat.size
};
writeFileSync(join(publicDir, 'wasm-info.json'), JSON.stringify(info, null, 2));
console.log(`[wasm] built public/spl.wasm (${wasmStat.size} bytes)`);
