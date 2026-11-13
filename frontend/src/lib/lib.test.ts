import { beforeEach, describe, expect, it } from 'vitest';
import { getApiRole } from '../api/auth';
import { useUi } from '../store/ui';
import { formatDate, formatDateTime, formatMinutes, formatMs, formatTime, initials, internContext, plural, prettyJson, todayIso } from './format';
import { groupByStage, stepMeta } from './onboarding';
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
    expect(JSON.parse(localStorage.getItem('ai-buddy-ui') ?? '{}').state).toEqual({ role: 'lead' });

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
});

describe('контекст и раскладка чек-листа', () => {
  it('собирает контекст стажёра для шапки', () => {
    const intern = { team: 'Autoteka', stack: 'Go', stage: 'Week 1', program: 'Стажировка Backend, 6 месяцев' };
    expect(internContext(intern)).toBe('Команда: Autoteka • Backend Go • Неделя 1 из 24');
    expect(internContext({ ...intern, program: 'Стажировка' })).toBe('Команда: Autoteka • Backend Go • Неделя 1');
    expect(internContext({ ...intern, stage: 'Онбординг' })).toBe('Команда: Autoteka • Backend Go • Онбординг');
  });

  it('форматирует инициалы и длительность вызова', () => {
    expect(initials('Мария Кузнецова')).toBe('МК');
    expect(initials('  Алексей  ')).toBe('А');
    expect(formatMs(0.4)).toBe('<1ms');
    expect(formatMs(139.6)).toBe('140ms');
  });

  it('группирует шаги по этапам и сервисам', () => {
    const item = (id: string, status: 'pending' | 'completed', scenario?: string) => ({
      id,
      position: 1,
      week: 1,
      title: id,
      description: '',
      status,
      scenario,
      updated_at: '2026-11-13T10:00:00+03:00',
    });
    const groups = groupByStage([item('access_repo', 'completed'), item('absence_rules', 'pending'), item('custom_step', 'pending', 'S2')]);
    expect(groups.map((g) => [g.id, g.items.length, g.completed])).toEqual([
      ['access', 1, 1],
      ['process', 1, 0],
      ['org', 1, 0],
    ]);
    expect(stepMeta({ id: 'absence_rules' })).toEqual({ stage: 'org', service: 'Отсутствия' });
    expect(stepMeta({ id: 'unknown' })).toEqual({ stage: 'org', service: 'Онбординг' });
  });
});
