import { beforeEach, describe, expect, it } from 'vitest';
import { getApiRole } from '../api/auth';
import { applyTheme, useUi } from '../store/ui';
import { formatDate, formatDateTime, formatMinutes, formatTime, plural, prettyJson, todayIso } from './format';
import { parseInline, parseRichText } from './richText';
import { absenceDefaults, chatInputSchema, validateAbsence } from './validation';

describe('richText', () => {
  it('выделяет жирный текст', () => {
    expect(parseInline('Доступ к **avito-autoteka-api** выдан')).toEqual([
      { text: 'Доступ к ', bold: false },
      { text: 'avito-autoteka-api', bold: true },
      { text: ' выдан', bold: false },
    ]);
    expect(parseInline('')).toEqual([]);
  });

  it('собирает абзацы и списки', () => {
    const blocks = parseRichText('**Заголовок**\n1. Первый\n2. Второй\n\n- пункт\n• ещё\nТекст');
    expect(blocks.map((b) => b.type)).toEqual(['paragraph', 'ordered', 'unordered', 'paragraph']);
    expect(blocks[1]).toMatchObject({ items: [[{ text: 'Первый' }], [{ text: 'Второй' }]] });
    expect(blocks[2]).toMatchObject({ items: [[{ text: 'пункт' }], [{ text: 'ещё' }]] });
    expect(parseRichText('  \n ')).toEqual([]);
  });

  it('не интерпретирует HTML', () => {
    expect(parseRichText('<img src=x onerror=alert(1)>')).toEqual([
      { type: 'paragraph', inlines: [{ text: '<img src=x onerror=alert(1)>', bold: false }] },
    ]);
  });
});

describe('format', () => {
  it('форматирует даты и время', () => {
    expect(formatDate('2026-11-13')).toBe('13.11.2026');
    expect(formatDate('скоро')).toBe('скоро');
    expect(formatTime('не дата')).toBe('');
    expect(formatTime(new Date(2026, 10, 13, 9, 5).toISOString())).toBe('09:05');
    expect(formatDateTime('2026-11-13')).toMatch(/^13\.11\.2026/);
    expect(formatDateTime('мусор')).toBe('мусор');
    expect(todayIso(new Date(2026, 0, 5))).toBe('2026-01-05');
  });

  it('склоняет и форматирует минуты', () => {
    const forms: [string, string, string] = ['шаг', 'шага', 'шагов'];
    expect([1, 2, 5, 11, 21, 22, 112].map((n) => plural(n, forms))).toEqual(['шаг', 'шага', 'шагов', 'шагов', 'шаг', 'шага', 'шагов']);
    expect(formatMinutes(45)).toBe('45 мин');
    expect(formatMinutes(120)).toBe('2 ч');
    expect(formatMinutes(100)).toBe('1 ч 40 мин');
  });

  it('prettyJson устойчив к циклам', () => {
    expect(prettyJson({ a: 1 })).toContain('"a": 1');
    const loop: Record<string, unknown> = {};
    loop.self = loop;
    expect(prettyJson(loop)).toBe('[object Object]');
    expect(prettyJson(undefined)).toBe('');
  });
});

describe('validation', () => {
  const valid = { intern_id: 'u-intern-01', date: '2026-11-20', time_from: '14:00', time_to: '18:00', reason: 'Зачёт', notify: ['mentor'] };

  it('принимает корректную заявку', () => {
    expect(validateAbsence(valid, '2026-11-13')).toEqual({ ok: true, values: valid });
    expect(validateAbsence({ ...valid, date: '2026-11-13' }, '2026-11-13').ok).toBe(true);
  });

  it.each([
    ['дата в прошлом', { date: '2026-11-01' }, 'date', 'уже прошла'],
    ['пустая дата', { date: '' }, 'date', 'Укажи дату'],
    ['конец раньше начала', { time_from: '18:00', time_to: '14:00' }, 'time_to', 'позже начала'],
    ['формат времени', { time_from: '25:00' }, 'time_from', 'ЧЧ:ММ'],
    ['короткая причина', { reason: ' a ' }, 'reason', 'минимум 3'],
    ['длинная причина', { reason: 'я'.repeat(301) }, 'reason', 'не больше 300'],
  ])('отклоняет: %s', (_name, patch, field, message) => {
    const result = validateAbsence({ ...valid, ...patch }, '2026-11-13');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.errors[field as keyof typeof result.errors]).toContain(message);
  });

  it('заполняет форму из карточки агента', () => {
    expect(absenceDefaults({ intern_id: 'u1', date: '2026-11-20', time_from: '14:00', time_to: '18:00', reason: 'Зачёт', notify: ['mentor', 'hr'] })).toEqual({
      intern_id: 'u1', date: '2026-11-20', time_from: '14:00', time_to: '18:00', reason: 'Зачёт', notify: ['mentor'],
    });
    expect(absenceDefaults({ date: 5 })).toEqual({ intern_id: '', date: '', time_from: '10:00', time_to: '19:00', reason: '', notify: ['mentor', 'team_lead'] });
  });

  it('проверяет сообщение чата', () => {
    expect(chatInputSchema.safeParse('  привет ').data).toBe('привет');
    expect(chatInputSchema.safeParse('   ').success).toBe(false);
    expect(chatInputSchema.safeParse('я'.repeat(2001)).success).toBe(false);
  });
});

describe('ui store', () => {
  beforeEach(() => {
    useUi.setState({ role: 'intern', inspectorOpen: false, inspectorTab: 'mcp', toast: null });
    useUi.getState().resetSession();
  });

  it('ведёт оптимистичный поток ответа', () => {
    const ui = () => useUi.getState();
    ui().pushStage({ kind: 'rbac', label: 'x' }); // без активной отправки — no-op
    ui().pushDelta('x');
    expect(ui().stream).toBeNull();

    ui().beginSend('Привет');
    ui().pushStage({ kind: 'rbac', label: 'Проверяю права…' });
    ui().pushDelta('Гото');
    ui().pushDelta('во');
    expect(ui()).toMatchObject({ pendingText: 'Привет', stream: { stages: [{ kind: 'rbac' }], text: 'Готово' } });
    ui().endSend();
    expect(ui()).toMatchObject({ pendingText: null, stream: null });

    ui().beginSend('Ещё');
    ui().failSend('Нет связи', 'Ещё');
    expect(ui()).toMatchObject({ stream: null, chatError: { message: 'Нет связи', retryText: 'Ещё' } });
    ui().beginSend('Ещё');
    expect(ui().chatError).toBeNull();
    ui().failSend('x', 'y');
    ui().clearChatError();
    expect(ui().chatError).toBeNull();
  });

  it('переключает роль, инспектор и уведомления', () => {
    const ui = () => useUi.getState();
    ui().beginSend('в процессе');
    ui().setRole('lead');
    expect(ui()).toMatchObject({ role: 'lead', pendingText: null });
    expect(getApiRole()).toBe('lead');
    expect(JSON.parse(localStorage.getItem('ai-buddy-ui') ?? '{}').state).toMatchObject({ role: 'lead' });

    ui().openInspector('rbac');
    expect(ui()).toMatchObject({ inspectorOpen: true, inspectorTab: 'rbac' });
    ui().openInspector();
    expect(ui().inspectorTab).toBe('rbac');
    ui().setInspectorTab('registry');
    ui().closeInspector();
    expect(ui()).toMatchObject({ inspectorOpen: false, inspectorTab: 'registry' });

    ui().showToast({ kind: 'success', text: 'ок' });
    expect(ui().toast?.text).toBe('ок');
    ui().hideToast();
    ui().setCronResult(null);
    expect(ui()).toMatchObject({ toast: null, cronResult: null });
    ui().setRole('intern');
  });

  it('переключает тему, применяет её к документу и сохраняет выбор', () => {
    const ui = () => useUi.getState();
    useUi.setState({ theme: 'light' });
    applyTheme('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    ui().toggleTheme();
    expect(ui().theme).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(JSON.parse(localStorage.getItem('ai-buddy-ui') ?? '{}').state.theme).toBe('dark');
    ui().toggleTheme();
    expect(ui().theme).toBe('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('запоминает показанные напоминания без дублей', () => {
    const ui = () => useUi.getState();
    ui().markRemindersSeen(['a', 'b']);
    ui().markRemindersSeen(['b', 'c']);
    expect(ui().seenReminders).toEqual(['a', 'b', 'c']);
    ui().resetSession();
    expect(ui().seenReminders).toEqual([]);
  });
});
