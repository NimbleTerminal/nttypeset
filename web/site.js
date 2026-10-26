import { BoobaTerminal, installActionBar, installRendererHud, parseRendererFromURL } from './booba/booba.js';

const demos = { typeset: 'Typeset a page', specimen: 'Font specimen', hackernews: 'Hacker News' };
const help = {
  hackernews: 'j/k or arrows select a story · Enter or o opens it · c opens comments · y copies the URL · r refreshes · +/= zooms in · - zooms out · 0 resets zoom · space/PgUp page · g/G ends · Q keeps the screen · q quits. Fetches the public Hacker News API directly from your browser.',
  typeset: 'j/k or arrows scroll · space and PgUp page · g/G ends · d dark · f next look · m mono · t glyph/Kitty · n numbers · c chop · drag selects · y copies · Esc clears · Q keeps the screen · q quits. Click the terminal to focus.',
  specimen: 'd dark · m mono · t glyph/Kitty · Q keeps the screen · q quits. Click the terminal to focus.',
};
const requested = new URLSearchParams(location.search).get('demo');
const demo = Object.hasOwn(demos, requested) ? requested : 'typeset';
const status = document.querySelector('#status');
document.querySelector('#title').textContent = demos[demo];
document.title = `${demos[demo]} · nttypeset`;
const restart = new URL(location.href);
restart.searchParams.set('demo', demo);
document.querySelector('#restart').href = restart.href;
document.querySelector('#source').href = `https://github.com/NimbleTerminal/nttypeset/tree/main/examples/${demo}`;
for (const link of document.querySelectorAll('[data-demo]')) {
  if (link.dataset.demo === demo) link.setAttribute('aria-current', 'page');
}
document.querySelector('#help').textContent = help[demo];
function failed(error) {
  status.dataset.error = '';
  status.textContent = `Could not run the demo: ${error.message}. Try Restart.`;
  console.error(error);
}
// Go's browser build calls this host function for navigation. Keep a real
// anchor available because browser popup policies vary for asynchronous input.
globalThis.nttypesetOpenURL = raw => {
  const url = new URL(raw);
  if (!['https:', 'http:', 'mailto:'].includes(url.protocol)) throw new Error('Unsupported link protocol');
  const fallback = document.querySelector('#opened-link');
  fallback.querySelector('a').href = url.href;
  fallback.hidden = false;
  window.open(url.href, '_blank', 'noopener,noreferrer');
};
async function main() {
  const go = new Go();
  status.textContent = 'Downloading the demo…';
  const response = await fetch(`demos/${demo}.wasm`);
  if (!response.ok) throw new Error(`Download failed (${response.status})`);
  const { instance } = await WebAssembly.instantiateStreaming(response, go.importObject);
  const running = go.run(instance);
  running.then(() => { status.textContent = 'Session ended. Choose Restart to run again.'; }, failed);
  const terminal = new BoobaTerminal('terminal', { renderer: parseRendererFromURL(), allowOSC7501Actions: true, allowOSC52: true });
  await terminal.init();
  terminal.setActionKeyEncoder({
    canEncode: gesture => terminal.encodeKey(gesture) !== null,
    encode: gesture => terminal.encodeKey(gesture),
  });
  installActionBar(terminal.actions, {
    parent: document.querySelector('#controls'),
    openKeyboard: () => terminal.focus(),
    onLayout: () => terminal.fitAddon?.fit(),
  });
  terminal.onActionError = failed;
  window.addEventListener('pagehide', () => terminal.dispose(), { once: true });
  terminal.connectWasm();
  terminal.focus();
  installRendererHud(terminal.term, {
    parent: document.querySelector('.terminal-frame'),
    position: 'absolute',
    bindToggleHotkey: false,
  });
  status.textContent = demo === 'hackernews'
    ? 'Running locally in your browser. Story data comes directly from the Hacker News API; press r to retry or refresh.'
    : 'Running locally in your browser. No terminal server required.';
}
main().catch(failed);
