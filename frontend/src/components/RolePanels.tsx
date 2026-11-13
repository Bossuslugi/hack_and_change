import type { ReactNode } from 'react';
import type { Brief, Notification, Workspace } from '../api/schemas';
import { useCron, useReviewAbsence, useSetFlag } from '../hooks/useBuddy';
import { ABSENCE_STATUS_LABEL, formatDate, formatDateTime, formatMinutes, plural } from '../lib/format';
import { Icon } from './ui/icons';
import { Badge, Button, Card, CardTitle, EmptyState, Eyebrow, FOCUS, cn, type Tone } from './ui/primitives';

/** Плитка метрики: подпись, крупное значение, пояснение. */
function Metric({ label, value, note, tone = 'neutral', children }: { label: string; value: string; note: string; tone?: Tone; children?: ReactNode }) {
  const color: Record<Tone, string> = {
    neutral: 'text-ink-900',
    accent: 'text-accent-700',
    ok: 'text-ok-700',
    ai: 'text-ai-700',
    warn: 'text-warn-700',
    danger: 'text-danger-700',
  };
  return (
    <Card className="flex flex-col p-4">
      <Eyebrow>{label}</Eyebrow>
      <p className={cn('mt-2 text-title font-semibold', color[tone])}>{value}</p>
      {children}
      <p className="mt-auto pt-2 text-meta text-ink-500">{note}</p>
    </Card>
  );
}

function SavedTime({ workspace }: { workspace: Workspace }) {
  const m = workspace.metrics;
  const share = Math.min(100, (m.mentor_minutes_saved / (m.mentor_hours_baseline_max * 60)) * 100);
  return (
    <Metric
      label="Сэкономлено наставнику"
      value={formatMinutes(m.mentor_minutes_saved)}
      tone="ok"
      note={`≈${m.saving_percent}% от ${m.mentor_hours_baseline_min}–${m.mentor_hours_baseline_max} ч наставника за программу. Оценка по весам из спецификации.`}
    >
      <div className="mt-2 h-2 overflow-hidden rounded-full bg-ink-900/10" aria-hidden="true">
        <div className="h-full rounded-full bg-ok-500" style={{ width: `${share}%` }} />
      </div>
    </Metric>
  );
}

function BriefCard({ brief }: { brief: Brief }) {
  return (
    <article className="rounded-card border border-line p-4" data-testid="brief">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-body font-semibold text-ink-900">
          {brief.intern_name} · <span className="font-mono text-meta font-normal text-ink-500">{brief.task_key}</span>
        </p>
        <span className="font-mono text-meta text-ink-400">{formatDateTime(brief.created_at)}</span>
      </div>

      <Eyebrow className="mt-4">Уже разобрал сам через AI-Buddy</Eyebrow>
      <ul className="mt-2 space-y-1 text-body text-ink-900">
        {brief.learned_topics.map((topic) => (
          <li key={topic} className="flex gap-2">
            <Icon name="check" className="mt-0.5 text-ok-500" strokeWidth="2.2" />
            {topic}
          </li>
        ))}
      </ul>

      <Eyebrow className="mt-4 text-ai-700">Вопрос, где нужен сеньор</Eyebrow>
      <p className="mt-2 rounded-card bg-ai-50 px-4 py-2 text-body text-ink-900">{brief.architecture_question}</p>

      {brief.signals.length > 0 && (
        <>
          <Eyebrow className="mt-4">Сигналы</Eyebrow>
          <ul className="mt-2 list-disc space-y-1 pl-4 text-meta text-ink-500 marker:text-ink-400">
            {brief.signals.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ul>
        </>
      )}
      {brief.open_questions.length > 0 && (
        <>
          <Eyebrow className="mt-4">Вопросы без ответа в базе знаний</Eyebrow>
          <ul className="mt-2 list-disc space-y-1 pl-4 text-meta text-ink-500 marker:text-ink-400">
            {brief.open_questions.map((q) => (
              <li key={q}>{q}</li>
            ))}
          </ul>
        </>
      )}
    </article>
  );
}

function Briefs({ workspace }: { workspace: Workspace }) {
  const cron = useCron();
  return (
    <Card className="p-4">
      <CardTitle aside={<Badge tone="ai">{workspace.briefs.length}</Badge>}>Structured 1:1 Brief</CardTitle>
      <p className="mt-1 text-meta text-ink-500">
        Бриф готовит ИИ-крон: что стажёр уже изучил сам и один вопрос, требующий сеньора. Переписка стажёра наставнику не раскрывается.
      </p>
      <div className="mt-4 space-y-2">
        {workspace.briefs.length === 0 ? (
          <EmptyState
            title="Брифов пока нет"
            hint="Крон сформирует бриф, когда задача стажёра зависнет в работе без коммитов."
            action={
              <Button variant="ai" onClick={() => cron.mutate()} loading={cron.isPending}>
                Симулировать ИИ-крон
              </Button>
            }
          />
        ) : (
          workspace.briefs.map((b) => <BriefCard key={b.id} brief={b} />)
        )}
      </div>
    </Card>
  );
}

function Notifications({ items }: { items: Notification[] }) {
  return (
    <Card className="p-4">
      <CardTitle aside={<Badge>{items.length}</Badge>}>Уведомления</CardTitle>
      <div className="mt-4 space-y-2">
        {items.length === 0 ? (
          <EmptyState title="Новых уведомлений нет" />
        ) : (
          items.map((n) => (
            <div key={n.id} className="rounded-card bg-canvas px-4 py-2" data-testid="notification">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-body font-medium text-ink-900">{n.title}</p>
                <span className="font-mono text-meta text-ink-400">{formatDateTime(n.created_at)}</span>
              </div>
              <p className="mt-1 text-meta text-ink-500">{n.body}</p>
            </div>
          ))
        )}
      </div>
    </Card>
  );
}

function Absences({ workspace }: { workspace: Workspace }) {
  const reviewAbsence = useReviewAbsence();
  const isLead = workspace.viewer.role === 'team_lead';

  return (
    <Card className="p-4">
      <CardTitle aside={<Badge>{workspace.absences.length}</Badge>}>Заявки на отсутствие</CardTitle>
      <div className="mt-4">
        {workspace.absences.length === 0 ? (
          <EmptyState title="Заявок нет" hint="Стажёр оформляет отгул в чате AI-Buddy через сервис «Отсутствия»." />
        ) : (
          <div className="scroll-thin overflow-x-auto">
            <table className="w-full text-left text-body">
              <thead className="font-mono text-meta uppercase tracking-wider text-ink-400">
                <tr>
                  <th className="py-2 pr-4 font-normal">Номер</th>
                  <th className="py-2 pr-4 font-normal">Сотрудник</th>
                  <th className="py-2 pr-4 font-normal">Когда</th>
                  <th className="py-2 pr-4 font-normal">Причина</th>
                  <th className="py-2 pr-4 font-normal">Статус</th>
                  {isLead && <th className="py-2 font-normal">Решение руководителя</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {workspace.absences.map((a) => {
                  const tone: Tone = a.status === 'approved' ? 'ok' : a.status === 'rejected' ? 'danger' : 'warn';
                  return (
                    <tr key={a.id} data-testid={`absence-row-${a.id}`}>
                      <td className="py-2 pr-4 font-mono text-meta font-medium text-ink-900">{a.number}</td>
                      <td className="py-2 pr-4 text-ink-900">{a.intern_name}</td>
                      <td className="py-2 pr-4 text-ink-500">
                        {formatDate(a.date)}
                        {a.time_from && `, ${a.time_from}–${a.time_to}`}
                      </td>
                      <td className="py-2 pr-4 text-ink-500">{a.reason}</td>
                      <td className="py-2 pr-4">
                        <Badge tone={tone}>{ABSENCE_STATUS_LABEL[a.status] ?? a.status}</Badge>
                      </td>
                      {isLead && (
                        <td className="py-2">
                          {a.status === 'pending_approval' ? (
                            <div className="flex items-center gap-2">
                              <button
                                type="button"
                                disabled={reviewAbsence.isPending}
                                onClick={() => reviewAbsence.mutate({ id: a.id, decision: 'approved' })}
                                aria-label={`Согласовать отгул ${a.number}`}
                                className={cn(
                                  'inline-flex h-7 items-center rounded-button bg-[#00C257] px-2.5 text-meta font-medium text-white transition-colors hover:bg-[#00A84B] disabled:opacity-50',
                                  FOCUS,
                                )}
                              >
                                Согласовать
                              </button>
                              <button
                                type="button"
                                disabled={reviewAbsence.isPending}
                                onClick={() => reviewAbsence.mutate({ id: a.id, decision: 'rejected' })}
                                aria-label={`Отклонить отгул ${a.number}`}
                                className={cn(
                                  'inline-flex h-7 items-center rounded-button border border-line px-2.5 text-meta font-medium text-ink-700 transition-colors hover:border-danger-500 hover:bg-danger-50 hover:text-danger-600 disabled:opacity-50',
                                  FOCUS,
                                )}
                              >
                                Отклонить
                              </button>
                            </div>
                          ) : (
                            <span className="text-meta text-ink-400">—</span>
                          )}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Card>
  );
}

function Incidents({ workspace }: { workspace: Workspace }) {
  return (
    <Card className="p-4">
      <CardTitle aside={<Badge tone={workspace.incidents.length > 0 ? 'danger' : 'ok'}>{workspace.incidents.length}</Badge>}>Инциденты безопасности</CardTitle>
      <div className="mt-4 space-y-2">
        {workspace.incidents.length === 0 ? (
          <EmptyState title="Инцидентов нет" hint="Сюда попадают попытки операций, запрещённых политикой RBAC (например, выгрузка прод-ПДн)." />
        ) : (
          workspace.incidents.map((e) => (
            <div key={e.id} className="rounded-card border border-danger-500/30 bg-danger-50 px-4 py-2" data-testid="incident">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="font-mono text-meta font-medium text-danger-700">{String(e.payload.operation ?? e.name)}</p>
                <span className="font-mono text-meta text-ink-400">{formatDateTime(e.created_at)}</span>
              </div>
              <p className="mt-1 text-meta text-ink-900">
                {String(e.payload.actor ?? e.actor_id)} ({e.actor_role}) · политика {String(e.payload.policy_id ?? '—')}
              </p>
              <p className="mt-1 text-meta text-ink-500">{String(e.payload.reason ?? '')}</p>
            </div>
          ))
        )}
      </div>
    </Card>
  );
}

function Flags({ workspace }: { workspace: Workspace }) {
  const setFlag = useSetFlag();
  const canWrite = workspace.permissions.flags_write === true;
  return (
    <Card className="p-4">
      <CardTitle>Feature flags</CardTitle>
      <p className="mt-1 text-meta text-ink-500">Runtime-флаги хранятся в Redis и применяются без перезапуска. Compile-time задаются при сборке.</p>
      <ul className="mt-2 divide-y divide-line">
        {workspace.feature_flags.map((f) => {
          const locked = f.kind !== 'runtime' || !canWrite;
          return (
            <li key={f.key} className="flex items-center justify-between gap-4 py-2">
              <div className="min-w-0">
                <p className="truncate font-mono text-meta font-medium text-ink-900">{f.key}</p>
                <p className="text-meta text-ink-500">{f.description}</p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Badge mono>{f.kind === 'runtime' ? 'runtime' : 'compile-time'}</Badge>
                <button
                  type="button"
                  role="switch"
                  aria-checked={f.enabled}
                  aria-label={f.key}
                  disabled={locked}
                  onClick={() => setFlag.mutate({ key: f.key, enabled: !f.enabled })}
                  className={cn(
                    'relative h-6 w-10 rounded-full transition-colors',
                    FOCUS,
                    f.enabled ? 'bg-ok-500' : 'bg-ink-400/50',
                    locked && 'cursor-not-allowed opacity-50',
                  )}
                >
                  <span className={cn('absolute top-0.5 h-5 w-5 rounded-full bg-white shadow-card transition-[left]', f.enabled ? 'left-[18px]' : 'left-0.5')} />
                </button>
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

function PanelHeader({ title, workspace, note }: { title: string; workspace: Workspace; note: string }) {
  return (
    <div className="px-2">
      <h1 className="text-title font-semibold text-ink-900">
        {title} · {workspace.viewer.name}
      </h1>
      <p className="mt-1 text-body text-ink-500">{note}</p>
    </div>
  );
}

/** Доля вопросов стажёра, закрытых без наставника; null — вопросов ещё не было. */
function deflectionRate(workspace: Workspace): number | null {
  const { questions_answered: answered, unanswered_questions: open } = workspace.metrics;
  return answered + open === 0 ? null : Math.round((answered / (answered + open)) * 100);
}

/** Правая зона наставника: сэкономленное время и бриф к 1:1 вместо переписки стажёра. */
export function MentorPanel({ workspace }: { workspace: Workspace }) {
  const m = workspace.metrics;
  return (
    <div className="scroll-thin min-h-0 flex-1 space-y-4 overflow-y-auto">
      <PanelHeader
        title="Кабинет наставника"
        workspace={workspace}
        note={`Стажёр ${workspace.intern.name}: рутину закрывает AI-Buddy, вам остаются вопросы, где нужен опыт.`}
      />
      <div className="grid grid-cols-3 gap-2">
        <SavedTime workspace={workspace} />
        <Metric
          label="Закрыто без вас"
          value={String(m.questions_answered)}
          note={`Вопросы стажёра, на которые ответил AI-Buddy. Действий в сервисах: ${m.actions_automated}.`}
        />
        <Metric label="Инструкций изучено" value={`${m.docs_read} / ${m.docs_total}`} note="Темы, которые не придётся объяснять на 1:1." tone="ai" />
      </div>
      <Briefs workspace={workspace} />
      <Notifications items={workspace.notifications} />
      <Absences workspace={workspace} />
    </div>
  );
}

/** Правая зона руководителя: метрики онбординга, безопасность, отсутствия и флаги. */
export function LeadPanel({ workspace }: { workspace: Workspace }) {
  const m = workspace.metrics;
  const task = workspace.sprint_task;
  const deflection = deflectionRate(workspace);
  return (
    <div className="scroll-thin min-h-0 flex-1 space-y-4 overflow-y-auto">
      <PanelHeader title="Кабинет руководителя" workspace={workspace} note="Метрики онбординга, контроль отсутствий и безопасности." />
      <div className="grid grid-cols-3 gap-2">
        <SavedTime workspace={workspace} />
        <Metric
          label="Deflection Rate"
          value={deflection === null ? '—' : `${deflection}%`}
          tone="accent"
          note={
            deflection === null
              ? 'Стажёр ещё не задавал вопросов.'
              : `${m.questions_answered} из ${m.questions_answered + m.unanswered_questions} вопросов закрыто без наставника.`
          }
        />
        <Metric
          label="Time-to-First-PR"
          value={task.commits > 0 ? `${task.days_in_progress} дн.` : '—'}
          tone={task.commits > 0 ? 'ok' : 'warn'}
          note={`${task.key}: в работе ${task.days_in_progress} ${plural(task.days_in_progress, ['день', 'дня', 'дней'])}, коммитов ${task.commits}. События PR трекер не отдаёт.`}
        />
        <Metric label="Блокировок RBAC" value={String(m.rbac_blocks)} tone={m.rbac_blocks > 0 ? 'danger' : 'neutral'} note="Запросы, остановленные политикой до обработчика." />
        <Metric label="Действий через MCP" value={String(m.actions_automated)} note={`Всего вызовов инструментов: ${m.tool_calls}.`} tone="ai" />
        <Metric label="Чек-лист" value={`${m.checklist_percent}%`} note={`Средний ответ агента: ${Math.round(m.avg_response_ms)} мс.`} />
      </div>
      <div className="grid gap-4 2xl:grid-cols-2">
        <Absences workspace={workspace} />
        <Incidents workspace={workspace} />
      </div>
      <Notifications items={workspace.notifications} />
      <Flags workspace={workspace} />
      <Briefs workspace={workspace} />
    </div>
  );
}
