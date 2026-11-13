// Zod-схемы форм интерфейса. Правила повторяют контракт MCP-инструмента absences.create_day_off
// (specs/02_mcp_tools.openspec.yaml), чтобы ошибка была видна до отправки на сервер.
import { z } from 'zod';
import { todayIso } from './format';

export const MAX_MESSAGE_LENGTH = 2000;

export const chatInputSchema = z
  .string()
  .trim()
  .min(1, 'Напиши вопрос')
  .max(MAX_MESSAGE_LENGTH, `Не больше ${MAX_MESSAGE_LENGTH} символов`);

const time = z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, 'Время в формате ЧЧ:ММ');

export const absenceFormSchema = (today: string = todayIso()) =>
  z
    .object({
      intern_id: z.string().min(1),
      date: z
        .string()
        .regex(/^\d{4}-\d{2}-\d{2}$/, 'Укажи дату отгула')
        .refine((value) => value >= today, 'Дата отгула уже прошла'),
      time_from: time,
      time_to: time,
      reason: z.string().trim().min(3, 'Причина — минимум 3 символа').max(300, 'Причина — не больше 300 символов'),
      notify: z.array(z.enum(['mentor', 'team_lead'])),
    })
    .refine((value) => value.time_to > value.time_from, {
      message: 'Окончание должно быть позже начала',
      path: ['time_to'],
    });

export type AbsenceFormValues = z.infer<ReturnType<typeof absenceFormSchema>>;

export type FieldErrors = Partial<Record<keyof AbsenceFormValues, string>>;

/** Проверяет форму отгула и возвращает значения либо ошибки по полям. */
export function validateAbsence(
  input: unknown,
  today?: string,
): { ok: true; values: AbsenceFormValues } | { ok: false; errors: FieldErrors } {
  const result = absenceFormSchema(today).safeParse(input);
  if (result.success) return { ok: true, values: result.data };
  const errors: FieldErrors = {};
  for (const issue of result.error.issues) {
    const field = issue.path[0] as keyof AbsenceFormValues | undefined;
    if (field && !errors[field]) errors[field] = issue.message;
  }
  return { ok: false, errors };
}

/** Приводит поля карточки агента к значениям формы (устойчиво к неполным данным). */
export function absenceDefaults(fields: Record<string, unknown>): AbsenceFormValues {
  const str = (key: string, fallback = '') => (typeof fields[key] === 'string' ? (fields[key] as string) : fallback);
  const notify = Array.isArray(fields.notify)
    ? fields.notify.filter((v): v is 'mentor' | 'team_lead' => v === 'mentor' || v === 'team_lead')
    : (['mentor', 'team_lead'] as const).slice();
  return {
    intern_id: str('intern_id'),
    date: str('date'),
    time_from: str('time_from', '10:00'),
    time_to: str('time_to', '19:00'),
    reason: str('reason'),
    notify,
  };
}
