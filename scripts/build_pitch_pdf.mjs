#!/usr/bin/env node
// Собирает самодостаточный питч-дек и PDF.
//   docs/presentation/pitch_deck.src.html  (исходник, редактируется руками)
//     → docs/presentation/pitch_deck.html  (скриншоты в base64, QR, ссылки — открывается где угодно)
//     → Avito_Start_AI_Buddy_Presentation.pdf (+ копия в docs/presentation/), 1920×1080, через Playwright.
//
//   cd scripts && npm install        # один раз: playwright-core (используется системный Edge/Chrome)
//   node scripts/build_pitch_pdf.mjs [--theme=light] [--no-pdf]
//
// Скриншоты берутся из docs/presentation/screenshots/ (см. capture_slides_screenshots.mjs).
// Картинки с data-asset="путь/от/корня" (маскот — frontend/public/goffy.png) тоже вшиваются в base64.
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright-core');

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pres = join(root, 'docs', 'presentation');
const srcPath = join(pres, 'pitch_deck.src.html');
const outHtml = join(pres, 'pitch_deck.html');
const outRoot = join(root, 'Avito_Start_AI_Buddy_Presentation.pdf');
const outDocs = join(pres, 'Avito_Start_AI_Buddy_Presentation.pdf');
const args = process.argv.slice(2);
const theme = (args.find((a) => a.startsWith('--theme=')) || '--theme=dark').split('=')[1];
const MAX_W = 2400; // ширина встраиваемых скриншотов (≈2× от размера на слайде)

function repoUrl() {
  try {
    return execFileSync('git', ['remote', 'get-url', 'origin'], { cwd: root, encoding: 'utf8' })
      .trim()
      .replace(/^git@([^:]+):/, 'https://$1/')
      .replace(/^ssh:\/\/git@([^/]+)\//, 'https://$1/')
      .replace(/^https?:\/\/[^@/]+@/, 'https://')
      .replace(/\.git$/, '')
      .replace(/\/$/, '');
  } catch {
    return null;
  }
}

async function launch() {
  if (process.env.CHROME_PATH) return chromium.launch({ executablePath: process.env.CHROME_PATH });
  for (const channel of ['msedge', 'chrome']) {
    try {
      return await chromium.launch({ channel });
    } catch {
      /* следующий */
    }
  }
  throw new Error('Не найден Edge/Chrome. Укажите путь в CHROME_PATH.');
}

let html = readFileSync(srcPath, 'utf8');

// 1. Репозиторий и QR
const repo = repoUrl();
if (repo) html = html.replace(/const DEF_REPO='[^']*';/, `const DEF_REPO='${repo}';`);
const currentRepo = html.match(/const DEF_REPO='([^']*)';/)[1];
const fnSrc = html.slice(html.indexOf('function makeQR('), html.indexOf('/* ---------- Состояние'));
const makeQR = new Function(`${fnSrc}; return makeQR;`)();
const qr = makeQR(currentRepo);
html = html.replace(/(<div data-qr="repo">)[\s\S]*?(<\/div>)/g, `$1${qr}$2`);
console.log('repo:', currentRepo, '· QR:', (html.match(/<div data-qr="repo">/g) || []).length);

const browser = await launch();
try {
  // 2. Скриншоты → base64 JPEG (вшиваются прямо в HTML и PDF)
  const shots = [...new Set([...html.matchAll(/data-shot="([^"]+)"/g)].map((m) => m[1]))];
  const page = await browser.newPage();
  const cache = {};
  for (const name of shots) {
    const file = join(pres, 'screenshots', name);
    if (!existsSync(file)) throw new Error(`Нет скриншота ${file} — запустите scripts/capture_slides_screenshots.mjs`);
    const png = readFileSync(file).toString('base64');
    cache[name] = await page.evaluate(
      async ({ png, maxW }) => {
        const im = new Image();
        im.src = 'data:image/png;base64,' + png;
        await im.decode();
        const k = Math.min(1, maxW / im.naturalWidth);
        const c = document.createElement('canvas');
        c.width = Math.round(im.naturalWidth * k);
        c.height = Math.round(im.naturalHeight * k);
        const g = c.getContext('2d');
        g.imageSmoothingQuality = 'high';
        g.drawImage(im, 0, 0, c.width, c.height);
        return c.toDataURL('image/jpeg', 0.88);
      },
      { png, maxW: MAX_W },
    );
    console.log(`  ${name}: ${(cache[name].length / 1024).toFixed(0)} КБ`);
  }
  html = html.replace(/(<img\b[^>]*?\bdata-shot="([^"]+)"[^>]*?\bsrc=")[^"]*(")/g, (m, a, name, b) => a + cache[name] + b);
  const left = [...html.matchAll(/<img\b[^>]*\bsrc="screenshots\//g)].length;
  if (left) throw new Error(`${left} скриншот(ов) не вшиты в HTML`);
  await page.close();

  // 3. Ассеты из репозитория (маскот Гоффи — frontend/public/goffy.png) → base64
  const mime = { png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', webp: 'image/webp', svg: 'image/svg+xml' };
  html = html.replace(/(<img\b[^>]*?\bdata-asset="([^"]+)"[^>]*?\bsrc=")[^"]*(")/g, (m, a, rel, b) => {
    const file = join(root, rel);
    if (!existsSync(file)) throw new Error(`Нет файла ${rel}`);
    const ext = rel.split('.').pop().toLowerCase();
    console.log('  ассет:', rel);
    return a + `data:${mime[ext] || 'application/octet-stream'};base64,` + readFileSync(file).toString('base64') + b;
  });

  html = html.replace(/<!--\s*\n\s*ИСХОДНИК[\s\S]*?-->/, '<!-- Сгенерировано scripts/build_pitch_pdf.mjs из pitch_deck.src.html — правьте исходник. -->');
  writeFileSync(outHtml, html);
  console.log('HTML:', outHtml, `(${(html.length / 1024 / 1024).toFixed(1)} МБ)`);

  if (!args.includes('--no-pdf')) {
    // 4. PDF через Playwright
    const p = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
    await p.goto(`${pathToFileURL(outHtml).href}?theme=${theme}`, { waitUntil: 'networkidle' });
    await p.evaluate(() => document.fonts.ready);
    const broken = await p.evaluate(() =>
      [...document.images]
        .filter((i) => !i.complete || i.naturalWidth === 0 || !i.src.startsWith('data:'))
        .map((i) => i.alt || i.src.slice(0, 40)),
    );
    if (broken.length) throw new Error('Не загрузились картинки: ' + broken.join(', '));
    const n = await p.evaluate(() => document.querySelectorAll('img[data-shot]').length);
    console.log('скриншотов на слайдах:', n, '— все загружены');
    await p.emulateMedia({ media: 'print' });
    await p.pdf({ path: outRoot, width: '1920px', height: '1080px', printBackground: true, preferCSSPageSize: true });
    copyFileSync(outRoot, outDocs);
    console.log('PDF:', outRoot);
    console.log('PDF:', outDocs);
  }
} finally {
  await browser.close();
}
