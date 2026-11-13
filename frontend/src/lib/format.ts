import type { ChecklistStatus } from '../api/schemas';

const pad = (n: number) => String(n).padStart(2, '0');

/** «14:05» из ISO-времени; для некорректной даты — пустая строка. */
export function formatTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** «13.11.2026» из «2026-11-13» или ISO-времени. */
export function formatDate(value: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(value);
  return m ? `${m[3]}.${m[2]}.${m[1]}` : value;
}

export function formatDateTime(iso: string): string {
  const time = formatTime(iso);
  return time ? `${formatDate(iso)}, ${time}` : formatDate(iso);
}

/** Склонение: plural(3, ['шаг', 'шага', 'шагов']) → «шага». */
export function plural(n: number, forms: [string, string, string]): string {
  const abs = Math.abs(n) % 100;
  const last = abs % 10;
  if (abs > 10 && abs < 20) return forms[2];
  if (last > 1 && last < 5) return forms[1];
  if (last === 1) return forms[0];
  return forms[2];
}

/** «1 ч 40 мин» из минут. */
export function formatMinutes(minutes: number): string {
  if (minutes < 60) return `${minutes} мин`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m === 0 ? `${h} ч` : `${h} ч ${m} мин`;
}

export const STATUS_LABEL: Record<ChecklistStatus, string> = {
  pending: 'Впереди',
  in_progress: 'В работе',
  completed: 'Выполнено',
};

export const ENGINE_LABEL: Record<string, string> = {
  smart: 'Smart Engine',
  llm: 'LLM',
  cron: 'ИИ-крон',
};

export const TOOL_STATUS_LABEL: Record<string, string> = {
  success: 'успех',
  forbidden: 'запрещено',
  invalid: 'невалидно',
  error: 'ошибка',
};

export const ABSENCE_STATUS_LABEL: Record<string, string> = {
  pending_approval: 'Ожидает согласования',
};

/** Сегодняшняя дата в формате YYYY-MM-DD (локальное время). */
export function todayIso(now: Date = new Date()): string {
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

/** Компактный JSON для инспектора. */
export function prettyJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? '';
  } catch {
    return String(value);
  }
}
