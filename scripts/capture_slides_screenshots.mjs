#!/usr/bin/env node
// Снимает реальные скриншоты живого приложения для питч-дека (Playwright + системный Edge/Chrome).
//
//   cd scripts && npm install          # один раз: playwright-core (браузер не скачивается)
//   node scripts/capture_slides_screenshots.mjs [--video] [--url=http://localhost:5173]
//
// Если фронтенд не отвечает, скрипт сам поднимает backend (go run ./cmd/server) и Vite.
// Результат: docs/presentation/screenshots/*.png (1600×900, deviceScaleFactor 2 → 3200×1800).
// С флагом --video та же прогулка по сценариям пишется в docs/demo_video.webm
// (кадры — CDP-скринкаст браузера, кодирование — встроенный MediaRecorder, ffmpeg не нужен).
import { spawn } from 'node:child_process';
import { mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright-core');

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const outDir = join(root, 'docs', 'presentation', 'screenshots');
const args = process.argv.slice(2);
const APP = (args.find((a) => a.startsWith('--url=')) || '--url=http://localhost:5173').slice(6);
const API = process.env.BACKEND_URL || 'http://127.0.0.1:8080';
const RECORD = args.includes('--video');
const VIEW = { width: 1600, height: 900 };

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const up = async (url) => {
  try {
    const r = await fetch(url);
    return r.status < 500;
  } catch {
    return false;
  }
};

// ---------- 1. Приложение ----------
const children = [];
async function ensureApp() {
  if (await up(APP)) return;
  console.log('Приложение не запущено — поднимаю backend и frontend…');
  if (!(await up(`${API}/healthz`))) {
    children.push(spawn('go', ['run', './cmd/server'], { cwd: join(root, 'backend'), stdio: 'ignore', shell: process.platform === 'win32' }));
  }
  children.push(spawn('npx', ['vite', '--port', '5173', '--strictPort'], { cwd: join(root, 'frontend'), stdio: 'ignore', shell: process.platform === 'win32' }));
  for (let i = 0; i < 120; i++) {
    if ((await up(APP)) && (await up(`${API}/healthz`))) return;
    await sleep(1000);
  }
  throw new Error(`Приложение не поднялось: ${APP} / ${API}`);
}

// ---------- 2. Браузер ----------
async function launch() {
  const exe = process.env.CHROME_PATH;
  if (exe) return chromium.launch({ executablePath: exe });
  for (const channel of ['msedge', 'chrome']) {
    try {
      return await chromium.launch({ channel });
    } catch {
      /* пробуем следующий */
    }
  }
  throw new Error('Не найден Edge/Chrome. Укажите путь в CHROME_PATH.');
}

await ensureApp();
mkdirSync(outDir, { recursive: true });

const browser = await launch();
const ctx = await browser.newContext({
  viewport: VIEW,
  deviceScaleFactor: 2,
  locale: 'ru-RU',
  colorScheme: 'dark',
});
const page = await ctx.newPage();

// ---------- Видео: собираем кадры скринкаста ----------
const frames = [];
let cdp;
if (RECORD) {
  cdp = await ctx.newCDPSession(page);
  cdp.on('Page.screencastFrame', ({ data, metadata, sessionId }) => {
    frames.push({ data, t: metadata.timestamp });
    cdp.send('Page.screencastFrameAck', { sessionId }).catch(() => {});
  });
  await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 85, maxWidth: VIEW.width, maxHeight: VIEW.height, everyNthFrame: 1 });
}

/** Кодирует кадры в WebM внутри браузера (canvas.captureStream + MediaRecorder), сохраняя реальные паузы. */
async function encodeWebm(path) {
  const enc = await browser.newPage();
  await enc.exposeFunction('nextFrame', (i) => frames[i] || null);
  const b64 = await enc.evaluate(async ({ n, w, h }) => {
    const canvas = document.createElement('canvas');
    canvas.width = w;
    canvas.height = h;
    document.body.appendChild(canvas);
    const g = canvas.getContext('2d');
    const rec = new MediaRecorder(canvas.captureStream(30), { mimeType: 'video/webm;codecs=vp9', videoBitsPerSecond: 1_200_000 });
    const chunks = [];
    rec.ondataavailable = (e) => e.data.size && chunks.push(e.data);
    const done = new Promise((r) => (rec.onstop = r));
    const load = (src) => new Promise((r) => { const im = new Image(); im.onload = () => r(im); im.src = src; });
    rec.start(1000);
    let prev = null;
    for (let i = 0; i < n; i++) {
      const f = await window.nextFrame(i);
      const im = await load('data:image/jpeg;base64,' + f.data);
      if (prev !== null) await new Promise((r) => setTimeout(r, Math.min(2500, Math.max(0, (f.t - prev) * 1000))));
      g.drawImage(im, 0, 0, w, h);
      prev = f.t;
    }
    await new Promise((r) => setTimeout(r, 1500));
    rec.stop();
    await done;
    const buf = new Uint8Array(await new Blob(chunks, { type: 'video/webm' }).arrayBuffer());
    let s = '';
    for (let i = 0; i < buf.length; i += 0x8000) s += String.fromCharCode(...buf.subarray(i, i + 0x8000));
    return btoa(s);
  }, { n: frames.length, w: VIEW.width, h: VIEW.height });
  writeFileSync(path, Buffer.from(b64, 'base64'));
  await enc.close();
}

const btn = (name) => page.getByRole('button', { name });
const radio = (name) => page.getByRole('radio', { name });
const tab = (name) => page.getByRole('tab', { name });

/** Ждёт, пока Гоффи допишет ответ (стриминг) и сеть успокоится. */
async function waitReply() {
  await sleep(600);
  await page.waitForFunction(() => !document.querySelector('[data-testid="streaming"]'), null, { timeout: 30000 });
  await page.waitForLoadState('networkidle').catch(() => {});
  await sleep(900);
}
/** Закрывает всплывающие напоминания, чтобы они не перекрывали интерфейс. */
async function hideToasts() {
  for (const t of await page.getByRole('button', { name: 'Скрыть уведомление' }).all()) await t.click().catch(() => {});
  await sleep(250);
}
async function setDark(dark) {
  const sw = page.getByRole('switch', { name: 'Тёмная тема' });
  if ((await sw.getAttribute('aria-checked')) !== String(dark)) await sw.click();
  await sleep(500);
}
async function shot(name, opts = {}) {
  await sleep(400);
  await page.screenshot({ path: join(outDir, name), animations: 'disabled', ...opts });
  console.log('✓', name);
}

try {
  await page.goto(APP, { waitUntil: 'networkidle' });
  await btn('Сбросить демо').click();
  await waitReply();
  await setDark(true);
  await hideToasts();
  const leftScroll = page.getByLabel('Адаптация стажёра').locator('.stack-scroll');
  /** Прокручивает левую панель так, чтобы элемент с текстом оказался в нужной части экрана. */
  const scrollLeftTo = async (text, block = 'center') => {
    await page.getByLabel('Адаптация стажёра').getByText(text, { exact: false }).first()
      .evaluate((el, b) => el.scrollIntoView({ block: b }), block);
    await sleep(500);
  };

  // 01b — Алексей до сценария 1А: чек-лист 3/7, у активного шага «Доступы» раскрыт паспорт заявки (Stash / Redash).
  await scrollLeftTo('Проект / ресурс');
  await shot('01b_access_passport.png');
  await leftScroll.evaluate((el) => el.scrollTo(0, 0));

  // 01 — сценарий 1А: доступ выдан через MCP, карточка повторяет заявку, чек-лист → 4/7, слева цели и OKR.
  await btn(/Запросить доступы к Stash/).click();
  await waitReply();
  await hideToasts();
  await leftScroll.evaluate((el) => el.scrollTo(0, 0));
  await shot('01_workspace_alexey_dark.png');

  // 08 — мини-квиз на шаге «Отсутствия»: ответ в один клик, разбор от Гоффи.
  await btn('Мини-квиз').first().click();
  await sleep(500);
  await page.getByRole('button', { name: /Оформить заявку в сервисе/ }).first().click();
  await sleep(500);
  await page.getByRole('group', { name: /Мини-квиз/ }).first().scrollIntoViewIfNeeded();
  await sleep(400);
  await shot('08_quiz.png');
  await leftScroll.evaluate((el) => el.scrollTo(0, 0));

  // 02 — Мария (Python/ML, Модерация, неделя 3): MOD-842 и диалог о DVC / Airflow / A/B в Redash и Grafana.
  await radio(/Мария/).click();
  await sleep(2000);
  await hideToasts();
  await shot('02_workspace_maria_ml.png');

  // 03 — сценарий 1Б: запрос prod-ПДн заблокирован POL-SEC-001, открыт Инспектор жюри (RBAC).
  await radio(/Алексей/).click();
  await sleep(2000);
  await btn(/Проверить доступ к prod-БД/).click();
  await waitReply();
  await hideToasts();
  // Инспектор открывается поверх чата, поэтому карточку отказа снимаем отдельно.
  await shot('03a_rbac_chat_denied.png');
  await btn('Инспектор жюри').click();
  await sleep(700);
  await tab(/RBAC/).click();
  await sleep(700);
  await shot('03_rbac_blocked_inspector.png');
  await btn('Закрыть инспектор').click();
  await sleep(500);

  // 05b — сценарий 3: интерактивная карточка отгула в чате стажёра.
  await btn(/Оформить отгул/).first().click();
  await waitReply();
  await hideToasts();
  await shot('05b_absence_card.png');
  // Кнопка внутри карточки (чип «📅 Оформить отгул» под чатом называется так же).
  // Кнопка внутри карточки: у формы aria-label = заголовок карточки (композер чата — тоже <form>).
  await page.locator('form[aria-label="Заявка на отгул"]').getByRole('button', { name: 'Оформить отгул' }).last().click();
  await waitReply();

  // 05 — кабинет руководителя: заявка #ABS-2026-84 с кнопками «Согласовать» / «Отклонить».
  await radio('Руководитель').click();
  await sleep(2000);
  await hideToasts();
  await page.locator('table').first().evaluate((t) => {
    const box = t.closest('.overflow-x-auto');
    if (box) box.scrollLeft = box.scrollWidth;
  }).catch(() => {});
  await shot('05_absence_lead_approval.png');

  // 04 — наставник: ИИ-крон сформировал Structured 1:1 Brief по AUT-1423.
  await radio('Наставник').click();
  await sleep(2000);
  await page.locator('section').filter({ hasText: 'Structured 1:1 Brief' }).getByRole('button', { name: 'Запустить ИИ-крон' }).first().click();
  await waitReply();
  await tab('Статистика').click().catch(() => {});
  await sleep(700);
  await hideToasts();
  await shot('04_mentor_1on1_brief.png');
  await tab('Адаптация').click().catch(() => {});

  // 06 — стажёр, вкладка «Календарь и задачи»: две недели, напоминания, карточка созвона 1:1.
  await radio('Стажёр').click();
  await sleep(2000);
  await hideToasts();
  await tab('Календарь').click();
  await sleep(800);
  await shot('06b_calendar_panel.png');
  await btn(/Созвон 1:1 с наставником ?[0-9]/).first().click();
  await sleep(900);
  await shot('06_planner_14days.png');
  await btn('Закрыть карточку события').click().catch(() => page.keyboard.press('Escape'));
  await sleep(500);

  // 07 — вкладка «Статистика и метрики» в светлой теме Avito Clean (Deflection Rate, бенчмарки Q&A).
  for (const q of [/Выкатка под фича-флагом/]) {
    const chip = btn(q).first();
    if (await chip.count()) {
      await chip.click();
      await waitReply();
    }
  }
  await hideToasts();
  await tab('Статистика').click();
  await setDark(false);
  await hideToasts();
  await shot('07_stats_and_light_mode.png');
  await scrollLeftTo('Бенчмарки онбординга', 'start');
  await shot('07b_benchmarks_light.png');

  if (RECORD) {
    await setDark(true);
    await tab('Адаптация').click();
    await radio('Руководитель').click();
    await sleep(1800);
    await btn(/Согласовать отгул/).first().click().catch(() => {});
    await sleep(2000);
    await radio('Стажёр').click();
    await sleep(3000);
  }
  if (RECORD) {
    await cdp.send('Page.stopScreencast');
    console.log('кодирую видео из', frames.length, 'кадров…');
    await encodeWebm(join(root, 'docs', 'demo_video.webm'));
    console.log('✓ docs/demo_video.webm');
  }
} finally {
  await ctx.close();
  await browser.close();
  for (const c of children) c.kill();
}
console.log('Готово:', readdirSync(outDir).filter((f) => f.endsWith('.png')).length, 'скриншотов в', outDir);
