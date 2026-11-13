import { useState, type FormEvent, type ReactNode } from 'react';
import { ApiError } from '../../api/client';
import type { ActionCard } from '../../api/schemas';
import { errorText, useExecuteAction } from '../../hooks/useBuddy';
import { formatDate } from '../../lib/format';
import { absenceDefaults, validateAbsence, type AbsenceFormValues, type FieldErrors } from '../../lib/validation';
import { Icon } from '../ui/icons';
import { Badge, Button, cn } from '../ui/primitives';

const inputClass =
  'h-10 w-full rounded-card border border-line bg-white px-2 text-body text-ink-900 transition-colors focus:border-accent-500 focus:outline-none focus:ring-2 focus:ring-accent-50 disabled:bg-canvas disabled:text-ink-500';

function Field({ label, error, children, htmlFor }: { label: string; error?: string; children: ReactNode; htmlFor: string }) {
  return (
    <div>
      <label htmlFor={htmlFor} className="mb-1 block text-meta font-medium text-ink-500">
        {label}
      </label>
      {children}
      {error && (
        <p role="alert" className="mt-1 text-meta text-danger-700">
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
  // Номер заявки приходит в ответе сервиса; после перезагрузки он остаётся в карточке результата ниже.
  const number = execute.data?.message?.result_card?.lines.find((line) => line.label === 'Номер заявки')?.value;
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
    <form onSubmit={submit} noValidate aria-label={card.title} className="overflow-hidden rounded-card border border-line bg-white shadow-card">
      <div className="flex flex-wrap items-center gap-2 border-b border-line bg-canvas px-4 py-2">
        <Icon name="calendar" className="text-ink-500" />
        <p className="text-body font-semibold text-ink-900">{card.title}</p>
        <Badge mono className="bg-ink-900/5">
          Отсутствия
        </Badge>
        <Badge tone={submitted ? 'ok' : 'warn'} className="ml-auto">
          {submitted ? 'Отправлено' : 'Ждёт подтверждения'}
        </Badge>
      </div>

      <div className="space-y-4 p-4">
        <p className="text-meta text-ink-500">{card.description}</p>

        <div className="grid grid-cols-4 gap-2">
          <div className="col-span-2">
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

        <Field label="Причина" error={errors.reason} htmlFor={id('reason')}>
          <input id={id('reason')} type="text" className={inputClass} value={values.reason} disabled={submitted} maxLength={300} onChange={(e) => set('reason', e.target.value)} />
        </Field>

        <fieldset>
          <legend className="mb-1 text-meta font-medium text-ink-500">Уведомить</legend>
          <div className="flex flex-wrap gap-4 text-body text-ink-900">
            {(['mentor', 'team_lead'] as const).map((who) => (
              <label key={who} className={cn('inline-flex items-center gap-2', submitted && 'text-ink-500')}>
                <input type="checkbox" className="h-4 w-4 accent-accent-500" checked={values.notify.includes(who)} disabled={submitted} onChange={() => toggleNotify(who)} />
                {who === 'mentor' ? 'Наставника' : 'Руководителя'}
              </label>
            ))}
          </div>
        </fieldset>

        {serverError && (
          <p role="alert" className="rounded-tag bg-danger-50 px-2 py-2 text-meta text-danger-700">
            {serverError}
          </p>
        )}
      </div>

      <div className="flex min-h-14 flex-wrap items-center gap-4 border-t border-line px-4 py-2">
        {submitted ? (
          <p className="flex items-center gap-2 text-meta font-medium text-ok-700">
            <Icon name="check" strokeWidth="2.4" />
            <span>
              Заявка {number ? `${number} ` : ''}на {formatDate(values.date)} сохранена в сервисе «Отсутствия» и ждёт согласования.
            </span>
          </p>
        ) : (
          <>
            <Button type="submit" variant="primary" loading={execute.isPending}>
              {card.submit_label}
            </Button>
            <span className="font-mono text-meta text-ink-400">MCP · {card.tool}</span>
          </>
        )}
      </div>
    </form>
  );
}
