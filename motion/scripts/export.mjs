import {spawn} from 'node:child_process';
import {execFileSync} from 'node:child_process';
import {readdirSync, existsSync, statSync} from 'node:fs';
import {createRequire} from 'node:module';
import {setTimeout as sleep} from 'node:timers/promises';
import puppeteer from 'puppeteer-core';

const require = createRequire(import.meta.url);
const FFPROBE = require('@ffprobe-installer/ffprobe').path;
const FFMPEG = require('@ffmpeg-installer/ffmpeg').path;

const PORT = 9004;
const BASE_URL = `http://127.0.0.1:${PORT}/`;
const CHROME = '/usr/bin/google-chrome';
const TMP = new URL('./tmp-profile/', import.meta.url);
const OUT = new URL('../output/', import.meta.url);
const MP4 = new URL('./evt2sse.mp4', OUT);

const server = spawn('npx', ['vite', '--host', '127.0.0.1', '--port', String(PORT)], {
  detached: true,
  stdio: ['ignore', 'pipe', 'pipe'],
});
let serverLog = '';
server.stdout.on('data', (d) => (serverLog += d.toString()));
server.stderr.on('data', (d) => (serverLog += d.toString()));

async function waitForServer() {
  for (let i = 0; i < 60; i++) {
    try {
      const res = await fetch(BASE_URL);
      if (res.ok) return;
    } catch {
      /* not up yet */
    }
    await sleep(500);
  }
  throw new Error('vite dev server did not start');
}

function mp4Size() {
  try {
    return statSync(MP4.pathname).size;
  } catch {
    return -1;
  }
}

function mp4IsValid() {
  if (!existsSync(MP4.pathname)) return false;
  try {
    const out = execFileSync(
      FFPROBE,
      ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', MP4.pathname],
      {encoding: 'utf8'},
    );
    return parseFloat(out) > 1;
  } catch {
    return false;
  }
}

let browser;
try {
  browser = await puppeteer.launch({
    executablePath: CHROME,
    headless: 'new',
    userDataDir: TMP.pathname,
    args: ['--no-sandbox', '--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'],
    defaultViewport: {width: 1920, height: 1080},
  });
  const page = await browser.newPage();
  const errors = [];
  const clues = [];
  page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`));
  page.on('console', (m) => {
    clues.push(`[${m.type()}] ${m.text()}`);
    if (m.type() === 'error') errors.push(`console.error: ${m.text()}`);
  });

  await waitForServer();
  await page.goto(BASE_URL, {waitUntil: 'networkidle0', timeout: 60000});
  await page.waitForSelector('select._select_hvw7c_34', {timeout: 45000});
  await sleep(2000);

  const setSelect = await page.evaluate(() => {
    const selects = [...document.querySelectorAll('select')];
    const info = selects.map((s) => ({
      label: (s.closest('div')?.textContent ?? '').slice(0, 30),
      options: [...s.options].map((o) => o.text),
    }));

    const exporter = selects.find((s) => [...s.options].some((o) => o.text === 'Video (FFmpeg)'));
    if (exporter) {
      const opt = [...exporter.options].find((o) => o.text === 'Video (FFmpeg)');
      exporter.value = opt.value;
      exporter.dispatchEvent(new Event('change', {bubbles: true}));
    } else {
      return {info, error: 'exporter select VF not found'};
    }
    return {info};
  });
  console.log('selects:', JSON.stringify(setSelect.info, null, 1));
  if (setSelect.error) throw new Error(setSelect.error);

  await sleep(1200);

  const clicked = await page.evaluate(() => {
    const btn = [...document.querySelectorAll('button')].find(
      (b) => b.textContent.trim() === 'Render' && b.getBoundingClientRect().height > 0,
    );
    if (!btn) return false;
    btn.click();
    return true;
  });
  console.log('Render clicked:', clicked);

  const started = Date.now();
  let done = false;
  await sleep(75 * 1000); // laisser démarrer le rendu (40+ s de frames)
  console.log('pollling pour fin de rendu (mp4 valide)...');
  while (Date.now() - started < 14 * 60 * 1000) {
    await sleep(8000);
    const size = mp4Size();
    const valid = mp4IsValid();
    console.log(
      `+${((Date.now() - started) / 1000).toFixed(0)}s size=${size >= 0 ? size : '…'} valid=${valid}`,
    );
    if (valid) {
      console.log('Export terminé (mp4 valide).');
      done = true;
      break;
    }
  }
  console.log('done=', done);
  await sleep(3000);
  console.log('final size =', mp4Size(), 'valid =', mp4IsValid());

  console.log('\n--- output/ ---');
  for (const f of readdirSync(OUT.pathname)) {
    const st = statSync(new URL(f, OUT));
    console.log(`  ${f} ${st.isDirectory() ? '<dir>' : `(${st.size} bytes)`}`);
  }

  if (errors.length) {
    console.error('\nRUNTIME ERRORS:');
    for (const e of errors) console.error(' -', e);
  }
  console.log('\n--- console clues (last 40) ---');
  for (const c of clues.slice(-40)) console.log(' ', c);
  console.log('\n--- vite/ffmpeg server log (filtered) ---');
  for (const line of serverLog.split('\n')) {
    const l = line.toLowerCase();
    if (/ffmpeg|error|warn|fail|ffprobe|stream|h264|error while/i.test(l)) console.log('  ', line.trim());
  }
} finally {
  await browser?.close().catch(() => {});
  try {
    process.kill(-server.pid, 'SIGTERM');
  } catch {
    /* already gone */
  }
}