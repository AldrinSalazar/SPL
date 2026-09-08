// Lightweight SPL 2 syntax highlighter (lexical only, mirrors the spec).
// Returns an HTML string with escaped text + token spans; safe for innerHTML.
// Structural validation stays in the Go engine (diagnostics panel).

const HEADER_KW = 'spl';
const BLOCK_KW = new Set(['track', 'harmonics', 'noise', 'hit', 'end']);
const SECTION_KW = new Set(['spectrum', 'curve']);
// Spec: -?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?
const NUMBER_RE = /^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$/;

// Past this size, skip tokenizing (still escape, so the backdrop stays aligned).
export const HIGHLIGHT_LIMIT = 300_000;

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function highlightLine(line: string): string {
  // No strings in SPL: '#' always starts a comment running to end of line.
  const hash = line.indexOf('#');
  const code = hash === -1 ? line : line.slice(0, hash);
  const comment = hash === -1 ? '' : line.slice(hash);
  let out = '';
  for (const part of code.split(/(\s+)/)) {
    if (part === '' || /^\s+$/.test(part)) {
      out += escapeHtml(part);
      continue;
    }
    if (part === HEADER_KW) out += `<span class="tok-kw">${part}</span>`;
    else if (BLOCK_KW.has(part)) out += `<span class="tok-block">${part}</span>`;
    else if (SECTION_KW.has(part)) out += `<span class="tok-section">${part}</span>`;
    else if (NUMBER_RE.test(part)) out += `<span class="tok-num">${escapeHtml(part)}</span>`;
    else out += `<span class="tok-invalid">${escapeHtml(part)}</span>`;
  }
  if (comment) out += `<span class="tok-comment">${escapeHtml(comment)}</span>`;
  return out;
}

export function highlightSPL(source: string): string {
  if (source.length > HIGHLIGHT_LIMIT) return `${escapeHtml(source)}\n`;
  // Trailing '\n' keeps the backdrop height in sync with the textarea when
  // the source ends with a newline (final newlines create no extra line box).
  return `${source.split('\n').map(highlightLine).join('\n')}\n`;
}
