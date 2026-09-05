import {spawn} from 'node:child_process';
import {setTimeout as sleep} from 'node:timers/promises';
import puppeteer from 'puppeteer-core';

const PORT = 9003;
const BASE_URL = `http://127.0.0.1:${PORT}/`;
const CHROME = '/usr/bin/google-chrome';
const TMP = new URL('./tmp-profile/', import.meta.url);

const scenes = ['01-title', '02-problem', '03-architecture', '04-channels', '05-resilience', '06-ops'];

const server = spawn('npx', ['vite', '--host', '127.0.0.1', '--port', String(PORT)], {
  detached: true,
  stdio: 'ignore',
});
const bail = () => {
  try {
    process.kill(-server.pid, 'SIGTERM');
  } catch {
    /* already gone */
  }
};
process.on('exit', bail);

let browser;
try {
  for (let i = 0; i < 60; i++) {
    try {
      if ((await fetch(BASE_URL)).ok) break;
    } catch {
      /* not up yet */
    }
    await sleep(500);
  }

  browser = await puppeteer.launch({
    executablePath: CHROME,
    headless: 'new',
    userDataDir: TMP.pathname,
    args: ['--no-sandbox', '--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'],
    defaultViewport: {width: 1600, height: 1000},
  });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`));
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(`console.error: ${m.text()}`);
  });

  await page.goto(BASE_URL, {waitUntil: 'networkidle0', timeout: 60000});
  await page.waitForSelector('div._name_1bybk_104', {timeout: 45000});
  await sleep(2000);

  const found = await page.evaluate((names) => {
    const labels = [...document.querySelectorAll('div._name_1bybk_104')].map((e) => e.textContent);
    return names.map((n) => labels.includes(n));
  }, scenes);

  console.log('scènes trouvées dans la timeline :');
  for (let i = 0; i < scenes.length; i++) {
    console.log(`  ${found[i] ? 'OK ' : 'MISSING '} ${scenes[i]}`);
  }

  if (errors.length) {
    console.error('\nRUNTIME ERRORS:');
    for (const e of errors) console.error(' -', e);
    process.exitCode = 1;
  } else if (found.every(Boolean)) {
    console.log('\nOK: les 6 scènes sont chargées, aucune erreur runtime.');
  } else {
    console.error('\nÉCHEC: scènes manquantes.');
    process.exitCode = 1;
  }
} finally {
  await browser?.close().catch(() => {});
  bail();
}
