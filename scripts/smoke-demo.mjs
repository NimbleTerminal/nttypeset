// Exercise one real WASM program through booba's JS bridge in a fresh process.
// This checks startup/input/resize/exit, not browser canvas rendering.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { setTimeout as delay } from 'node:timers/promises';
const demo = process.argv[2];
assert.ok(['typeset', 'specimen', 'hackernews'].includes(demo));
vm.runInThisContext(await readFile('web/wasm_exec.js', 'utf8'));
const go = new Go();
let exitCode;
go.exit = code => { exitCode = code; };
const { instance } = await WebAssembly.instantiate(await readFile(`web/demos/${demo}.wasm`), go.importObject);
const timeout = setTimeout(() => { console.error(`${demo}: timed out`); process.exit(1); }, 15000);
// Exercise browser fetch deterministically; CI must not depend on the live API.
const requests = [];
const opened = [];
let failNextFetch = false;
if (demo === 'hackernews') {
  globalThis.fetch = async raw => {
    const url = new URL(raw);
    assert.equal(url.origin, 'https://hacker-news.firebaseio.com');
    requests.push(url.pathname);
    await delay(5);
    if (url.pathname === '/v0/topstories.json') {
      if (failNextFetch) {
        failNextFetch = false;
        return new Response('', { status: 503, statusText: 'Service Unavailable' });
      }
      return Response.json([1, 2, 3]);
    }
    const match = url.pathname.match(/^\/v0\/item\/([123])\.json$/);
    assert.ok(match, `unexpected API request: ${url}`);
    const id = Number(match[1]);
    return Response.json({ id, type: 'story', title: `Browser fixture story ${id}`,
      url: `https://example.com/${id}`, by: 'reader', score: 12, descendants: 3,
      time: Math.floor(Date.now() / 1000) - 60 });
  };
  globalThis.nttypesetOpenURL = raw => { opened.push(raw); };
}
// net/http deliberately disables Fetch when it detects Node. Hide process
// only during Go initialization so this smoke exercises the browser transport.
const hostProcess = globalThis.process;
if (demo === 'hackernews') globalThis.process = undefined;
let running;
try { running = go.run(instance); } finally { globalThis.process = hostProcess; }
// Poll the bridge until some output arrives after an input or resize.
async function outputFollows() {
  let out = '';
  for (let n = 0; n < 100 && out.length === 0; n++) {
    await delay(25);
    out += bubbletea_read();
  }
  return out;
}
try {
  for (let n = 0; typeof globalThis.bubbletea_read !== 'function' && n < 100; n++) await delay(20);
  assert.equal(typeof globalThis.bubbletea_read, 'function', 'booba bridge installed');
  // The help row always ends with Quit, even when it truncates the rest.
  bubbletea_resize(100, 40);
  let output = '';
  for (let n = 0; n < 100 && !output.includes('Quit'); n++) {
    await delay(25);
    output += bubbletea_read();
  }
  assert.ok(output.includes('Quit'), 'initial terminal view rendered');
  assert.ok(output.includes('\x1b]7501;?'), 'application queries action support');
  bubbletea_write('\x1b]7501;?\x1b\\');
  let report;
  for (let n = 0; n < 100 && !report; n++) {
    await delay(25);
    output += bubbletea_read();
    report = output.match(/\x1b\]7501;state=idle:[^\x1b]*:msg=([A-Za-z0-9+/=]+)\x1b\\/);
  }
  assert.ok(report, 'application publishes OSC action snapshot');
  const payload = Buffer.from(report[1], 'base64').toString('utf8');
  const snapshot = JSON.parse(payload.split(' | booba-actions/v1 ')[1]);
  const enabled = label => snapshot.actions.some(action => action.label === label && action.enabled);
  const quit = snapshot.actions.find(action => action.label === 'Quit');
  assert.ok(quit?.enabled, 'quit action enabled');
  // Node answers no Kitty query, so glyph output is active and the toggle
  // must not be advertised.
  assert.ok(!enabled('Glyph / Kitty'), 'graphics toggle hidden without Kitty');
  if (demo === 'typeset') {
    for (const label of ['Down', 'Next look', 'Dark / Light']) {
      assert.ok(enabled(label), `${label} action`);
    }
    bubbletea_write('j');
    assert.ok((await outputFollows()).length > 0, 'scrolling produces output');
    assert.ok(enabled('Copy'), 'copy action');
    bubbletea_write('\x1b[<0;5;2M');
    bubbletea_write('\x1b[<32;40;12M');
    assert.ok((await outputFollows()).length > 0, 'drag produces output');
    bubbletea_write('\x1b[<0;40;12m');
    bubbletea_write('y');
    let copied = '';
    for (let n = 0; n < 100 && !/\x1b\]52;c;[A-Za-z0-9+/=]+/.test(copied); n++) {
      await delay(25);
      copied += bubbletea_read();
    }
    assert.ok(/\x1b\]52;c;[A-Za-z0-9+/=]+/.test(copied), 'y writes the selection to the clipboard');
  } else if (demo === 'hackernews') {
    for (const label of ['Next story', 'Open story', 'Comments', 'Copy story URL', 'Refresh', 'Zoom in', 'Zoom out']) {
      assert.ok(enabled(label), `${label} action`);
    }
    async function waitForOutput(fragment) {
      let output = '';
      for (let n = 0; n < 120 && !output.includes(fragment); n++) {
        await delay(25);
        output += bubbletea_read();
      }
      assert.ok(output.includes(fragment), `output contains ${fragment}: ${JSON.stringify(output).slice(-2000)}`);
    }
    // Startup can render before the asynchronous request has completed.
    if (!output.includes('3 stories')) await waitForOutput('3 stories');
    assert.ok(requests.includes('/v0/topstories.json'));
    for (const id of [1, 2, 3]) assert.ok(requests.includes(`/v0/item/${id}.json`));
    // Terminal rendering sends cursor deltas, so observe the published action
    // state rather than searching for a fully repainted percentage string.
    async function waitForResetZoom(expected) {
      let output = '';
      for (let n = 0; n < 120; n++) {
        await delay(25);
        output += bubbletea_read();
        for (const report of output.matchAll(/\x1b\]7501;state=idle:[^\x1b]*:msg=([A-Za-z0-9+/=]+)\x1b\\/g)) {
          const payload = Buffer.from(report[1], 'base64').toString('utf8');
          const snapshot = JSON.parse(payload.split(' | booba-actions/v1 ')[1]);
          if (snapshot.actions.some(action => action.label === 'Reset zoom' && action.enabled) === expected) return;
        }
      }
      assert.fail(`Reset zoom enabled=${expected} was not published`);
    }
    bubbletea_write('=');
    await waitForResetZoom(true);
    bubbletea_write('-');
    await waitForResetZoom(false);
    bubbletea_write('+');
    await waitForResetZoom(true);
    bubbletea_write('0');
    await waitForResetZoom(false);
    bubbletea_write('j');
    await outputFollows();
    bubbletea_write('o');
    for (let n = 0; n < 100 && opened.length < 1; n++) await delay(25);
    assert.equal(opened[0], 'https://example.com/2', 'selected story opens through browser host');
    bubbletea_write('c');
    for (let n = 0; n < 100 && opened.length < 2; n++) await delay(25);
    assert.equal(opened[1], 'https://news.ycombinator.com/item?id=2', 'comments open through browser host');
    bubbletea_write('y');
    await waitForOutput('\x1b]52;c;');
    failNextFetch = true;
    bubbletea_write('r');
    await waitForOutput('503');
    bubbletea_write('r');
    // Terminal diffs may retain the shared 'refresh' prefix on screen.
    await waitForOutput('ed 3 stories');
    assert.equal(requests.filter(path => path === '/v0/topstories.json').length, 3, 'refresh retries after an API failure');
  } else {
    bubbletea_write('d');
    assert.ok((await outputFollows()).length > 0, 'dark toggle produces output');
  }
  bubbletea_resize(60, 20);
  assert.ok((await outputFollows()).length > 0, 'resize produces output');
  bubbletea_write(process.argv.includes('keep') ? 'Q' : quit.keys[quit.preferred ?? 0].key);
  await running;
  assert.equal(exitCode, 0);
  assert.equal(globalThis.bubbletea_exited, true, 'exit published to adapter');
  console.log(`${demo}: startup, action publication, resize/input, and clean exit passed`);
} finally {
  clearTimeout(timeout);
}
// Like go_js_wasm_exec, terminate the host process after Go exits. The Go
// runtime may still have scheduled timer callbacks in this Node process.
process.exit(0);
