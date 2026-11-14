import { useState, type FormEvent } from 'react';
import { ApiError } from '../../api/client';
import type { ActionCard } from '../../api/schemas';
import { errorText, useExecuteAction } from '../../hooks/useBuddy';
import { absenceDefaults, validateAbsence, type AbsenceFormValues, type FieldErrors } from '../../lib/validation';
import { formatDate } from '../../lib/format';
import { Badge, Button, cn } from '../ui/primitives';

const inputClass =
  'w-full rounded-xl border border-paper-200 bg-paper-0 px-2.5 py-1.5 text-sm text-ink-900 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-100 disabled:bg-paper-50 disabled:text-ink-500';

function Field({ label, error, children, htmlFor }: { label: string; error?: string; children: React.ReactNode; htmlFor: string }) {
  return (
    <div>
      <label htmlFor={htmlFor} className="mb-1 block text-xs font-medium text-ink-700">
        {label}
      </label>
      {children}
      {error && (
        <p role="alert" className="mt-1 text-[11px] text-danger-700">
          {error}
        </p>
      )}
    </div>
  );
}

/** Интерактивная карточка оформления отгула: действие выполняется только после подтверждения. */
export function AbsenceForm({ card, messageId }: { card: ActionCard; messageId: string }) {
  const [values, setValues] = useState<AbsenceFormValues>(() => absenceDefaults(card.fields));
  const [errors, setErrors] = useState<FieldErrors>({});
  const [serverError, setServerError] = useState<string | null>(null);
  const execute = useExecuteAction();
  const submitted = card.status === 'submitted' || execute.isSuccess;
  const id = (name: string) => `absence-${messageId}-${name}`;

  const set = <K extends keyof AbsenceFormValues>(key: K, value: AbsenceFormValues[K]) => {
    setValues((v) => ({ ...v, [key]: value }));
    setErrors((e) => ({ ...e, [key]: undefined }));
    setServerError(null);
  };

  const toggleNotify = (who: 'mentor' | 'team_lead') =>
    set('notify', values.notify.includes(who) ? values.notify.filter((n) => n !== who) : [...values.notify, who]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const checked = validateAbsence(values);
    if (!checked.ok) {
      setErrors(checked.errors);
      return;
    }
    setServerError(null);
    execute.mutate(
      { tool: card.tool, args: checked.values, messageId },
      {
        onError: (error) => {
          const rbac = error instanceof ApiError && error.rbac ? ` (политика ${error.rbac.policy_id})` : '';
          setServerError(errorText(error) + rbac);
        },
      },
    );
  };

  return (
    <form onSubmit={submit} noValidate aria-label={card.title} className="mt-3 rounded-2xl border border-brand-100 bg-brand-50/60 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-semibold text-ink-900">{card.title}</p>
        <Badge tone={submitted ? 'ok' : 'brand'}>{submitted ? 'Отправлено' : 'Ждёт подтверждения'}</Badge>
      </div>
      <p className="mt-0.5 text-[11px] text-ink-500">{card.description}</p>

      <div className="mt-3 grid grid-cols-2 gap-2.5 sm:grid-cols-3">
        <div className="col-span-2 sm:col-span-1">
          <Field label="Дата" error={errors.date} htmlFor={id('date')}>
            <input id={id('date')} type="date" className={inputClass} value={values.date} disabled={submitted} onChange={(e) => set('date', e.target.value)} />
          </Field>
        </div>
        <Field label="С" error={errors.time_from} htmlFor={id('from')}>
          <input id={id('from')} type="time" className={inputClass} value={values.time_from} disabled={submitted} onChange={(e) => set('time_from', e.target.value)} />
        </Field>
        <Field label="До" error={errors.time_to} htmlFor={id('to')}>
          <input id={id('to')} type="time" className={inputClass} value={values.time_to} disabled={submitted} onChange={(e) => set('time_to', e.target.value)} />
        </Field>
      </div>
      <div className="mt-2.5">
        <Field label="Причина" error={errors.reason} htmlFor={id('reason')}>
          <input id={id('reason')} type="text" className={inputClass} value={values.reason} disabled={submitted} maxLength={300} onChange={(e) => set('reason', e.target.value)} />
        </Field>
      </div>

      <fieldset className="mt-2.5">
        <legend className="mb-1 text-xs font-medium text-ink-700">Уведомить</legend>
        <div className="flex flex-wrap gap-3 text-sm text-ink-700">
          {(['mentor', 'team_lead'] as const).map((who) => (
            <label key={who} className={cn('inline-flex items-center gap-1.5', submitted && 'text-ink-500')}>
              <input type="checkbox" className="h-4 w-4 rounded accent-brand-500" checked={values.notify.includes(who)} disabled={submitted} onChange={() => toggleNotify(who)} />
              {who === 'mentor' ? 'Наставника' : 'Руководителя'}
            </label>
          ))}
        </div>
      </fieldset>

      {serverError && (
        <p role="alert" className="mt-3 rounded-xl bg-danger-50 px-2.5 py-2 text-xs text-danger-700">
          {serverError}
        </p>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-3">
        {submitted ? (
          <p className="text-xs text-ok-700">Заявка на {formatDate(values.date)} отправлена через сервис «Отсутствия».</p>
        ) : (
          <>
            <Button type="submit" variant="primary" loading={execute.isPending}>
              {card.submit_label}
            </Button>
            <span className="text-[11px] text-ink-400">Гоффи отправит заявку только после нажатия</span>
          </>
        )}
      </div>
    </form>
  );
}
