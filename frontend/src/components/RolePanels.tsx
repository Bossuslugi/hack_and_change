import type { Brief, Notification, Workspace } from '../api/schemas';
import { useCron, useReviewAbsence, useSetFlag } from '../hooks/useBuddy';
import { ABSENCE_STATUS_LABEL, formatDate, formatDateTime } from '../lib/format';
import { Badge, Button, Card, CardTitle, EmptyState, cn } from './ui/primitives';

function BriefCard({ brief }: { brief: Brief }) {
  return (
    <article className="rounded-xl border border-paper-200 p-4" data-testid="brief">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-semibold text-ink-900">
          {brief.intern_name} · <span className="font-mono text-xs text-ink-500">{brief.task_key}</span>
        </p>
        <span className="text-[11px] text-ink-400">{formatDateTime(brief.created_at)}</span>
      </div>

      <h3 className="mt-3 text-xs font-semibold uppercase tracking-wide text-ink-400">Уже разобрал сам через Гоффи</h3>
      <ul className="mt-1.5 space-y-1 text-sm text-ink-700">
        {brief.learned_topics.map((topic) => (
          <li key={topic} className="flex gap-2">
            <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-ok-500" aria-hidden="true" />
            {topic}
          </li>
        ))}
      </ul>

      <h3 className="mt-3 text-xs font-semibold uppercase tracking-wide text-brand-600">Вопрос, где нужен сеньор</h3>
      <p className="mt-1.5 rounded-lg bg-brand-50 px-3 py-2 text-sm leading-relaxed text-ink-900">{brief.architecture_question}</p>

      {brief.signals.length > 0 && (
        <>
          <h3 className="mt-3 text-xs font-semibold uppercase tracking-wide text-ink-400">Сигналы</h3>
          <ul className="mt-1.5 list-disc space-y-1 pl-5 text-xs text-ink-700 marker:text-ink-300">
            {brief.signals.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ul>
        </>
      )}
      {brief.open_questions.length > 0 && (
        <>
          <h3 className="mt-3 text-xs font-semibold uppercase tracking-wide text-ink-400">Вопросы без ответа в базе знаний</h3>
          <ul className="mt-1.5 list-disc space-y-1 pl-5 text-xs text-ink-700 marker:text-ink-300">
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
      <CardTitle aside={<Badge tone="brand">{workspace.briefs.length}</Badge>}>Structured 1:1 Brief</CardTitle>
      <p className="mt-1 text-xs text-ink-500">
        Бриф готовит ИИ-крон: что стажёр уже изучил сам и один вопрос, требующий сеньора. Переписка стажёра наставнику не раскрывается.
      </p>
      <div className="mt-3 space-y-3">
        {workspace.briefs.length === 0 ? (
          <EmptyState
            title="Брифов пока нет"
            hint="Крон сформирует бриф, когда задача стажёра зависнет в работе без коммитов."
            action={
              <Button variant="primary" onClick={() => cron.mutate()} loading={cron.isPending}>
                Запустить ИИ-крон
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
      <div className="mt-3 space-y-2">
        {items.length === 0 ? (
          <EmptyState title="Новых уведомлений нет" />
        ) : (
          items.map((n) => (
            <div key={n.id} className="rounded-xl bg-paper-50 px-3 py-2.5" data-testid="notification">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm font-medium text-ink-900">{n.title}</p>
                <span className="text-[11px] text-ink-400">{formatDateTime(n.created_at)}</span>
              </div>
              <p className="mt-1 text-xs leading-relaxed text-ink-700">{n.body}</p>
            </div>
          ))
        )}
      </div>
    </Card>
  );
}

function Absences({ workspace }: { workspace: Workspace }) {
  const review = useReviewAbsence();
  const canReview = workspace.permissions.absence_approve === true;
  return (
    <Card className="p-4">
      <CardTitle aside={<Badge>{workspace.absences.length}</Badge>}>Заявки на отсутствие</CardTitle>
      <div className="mt-3">
        {workspace.absences.length === 0 ? (
          <EmptyState title="Заявок нет" hint="Стажёр оформляет отгул в чате Гоффи через сервис «Отсутствия»." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-[11px] uppercase tracking-wide text-ink-400">
                <tr>
                  <th className="py-1.5 pr-3 font-medium">Номер</th>
                  <th className="py-1.5 pr-3 font-medium">Сотрудник</th>
                  <th className="py-1.5 pr-3 font-medium">Когда</th>
                  <th className="py-1.5 pr-3 font-medium">Причина</th>
                  <th className="py-1.5 font-medium">Статус</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-paper-100">
                {workspace.absences.map((a) => (
                  <tr key={a.id}>
                    <td className="py-2 pr-3 font-mono text-xs font-medium text-ink-900">{a.number}</td>
                    <td className="py-2 pr-3 text-ink-700">{a.intern_name}</td>
                    <td className="py-2 pr-3 text-ink-700">
                      {formatDate(a.date)}
                      {a.time_from && `, ${a.time_from}–${a.time_to}`}
                    </td>
                    <td className="py-2 pr-3 text-ink-700">{a.reason}</td>
                    <td className="py-2">
                      <span className="flex flex-wrap items-center gap-2">
                        <Badge tone={a.status === 'approved' ? 'ok' : a.status === 'rejected' ? 'danger' : 'warn'}>
                          {ABSENCE_STATUS_LABEL[a.status] ?? a.status}
                        </Badge>
                        {canReview && a.status === 'pending_approval' && (
                          <>
                            <Button
                              size="sm"
                              variant="primary"
                              loading={review.isPending}
                              aria-label={`Согласовать отгул ${a.number}`}
                              onClick={() => review.mutate({ id: a.id, decision: 'approved' })}
                            >
                              Согласовать
                            </Button>
                            <Button
                              size="sm"
                              variant="danger"
                              disabled={review.isPending}
                              aria-label={`Отклонить отгул ${a.number}`}
                              onClick={() => review.mutate({ id: a.id, decision: 'rejected' })}
                            >
                              Отклонить
                            </Button>
                          </>
                        )}
                      </span>
                    </td>
                  </tr>
                ))}
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
      <div className="mt-3 space-y-2">
        {workspace.incidents.length === 0 ? (
          <EmptyState title="Инцидентов нет" hint="Сюда попадают попытки операций, запрещённых политикой RBAC (например, выгрузка прод-ПДн)." />
        ) : (
          workspace.incidents.map((e) => (
            <div key={e.id} className="rounded-xl border border-danger-500/20 bg-danger-50 px-3 py-2.5" data-testid="incident">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="font-mono text-xs font-semibold text-danger-700">{String(e.payload.operation ?? e.name)}</p>
                <span className="text-[11px] text-ink-400">{formatDateTime(e.created_at)}</span>
              </div>
              <p className="mt-1 text-xs text-ink-700">
                {String(e.payload.actor ?? e.actor_id)} ({e.actor_role}) · политика {String(e.payload.policy_id ?? '—')}
              </p>
              <p className="mt-0.5 text-xs text-ink-500">{String(e.payload.reason ?? '')}</p>
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
      <p className="mt-1 text-xs text-ink-500">Runtime-флаги хранятся в Redis и применяются без перезапуска. Compile-time задаются при сборке.</p>
      <ul className="mt-3 divide-y divide-paper-100">
        {workspace.feature_flags.map((f) => {
          const locked = f.kind !== 'runtime' || !canWrite;
          return (
            <li key={f.key} className="flex items-center justify-between gap-3 py-2.5">
              <div className="min-w-0">
                <p className="truncate font-mono text-xs font-medium text-ink-900">{f.key}</p>
                <p className="text-xs text-ink-500">{f.description}</p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Badge>{f.kind === 'runtime' ? 'runtime' : 'compile-time'}</Badge>
                <button
                  type="button"
                  role="switch"
                  aria-checked={f.enabled}
                  aria-label={f.key}
                  disabled={locked}
                  onClick={() => setFlag.mutate({ key: f.key, enabled: !f.enabled })}
                  className={cn(
                    'relative h-6 w-10 rounded-full transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500',
                    f.enabled ? 'bg-ok-500' : 'bg-ink-300',
                    locked && 'cursor-not-allowed opacity-50',
                  )}
                >
                  <span className={cn('absolute top-0.5 h-5 w-5 rounded-full bg-paper-0 shadow transition-[left]', f.enabled ? 'left-[18px]' : 'left-0.5')} />
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
    <div>
      <h1 className="text-lg font-semibold text-ink-900">
        {title} · {workspace.viewer.name}
      </h1>
      <p className="mt-0.5 text-sm text-ink-500">{note}</p>
    </div>
  );
}

export function MentorPanel({ workspace }: { workspace: Workspace }) {
  return (
    <div className="space-y-4">
      <PanelHeader
        title="Кабинет наставника"
        workspace={workspace}
        note={`Стажёр ${workspace.intern.name}: рутину закрывает Гоффи, вам остаются вопросы, где нужен опыт.`}
      />
      <Briefs workspace={workspace} />
      <Notifications items={workspace.notifications} />
      <Absences workspace={workspace} />
    </div>
  );
}

export function LeadPanel({ workspace }: { workspace: Workspace }) {
  return (
    <div className="space-y-4">
      <PanelHeader title="Кабинет руководителя" workspace={workspace} note="Контроль важных шагов адаптации, отсутствий и безопасности." />
      <div className="grid gap-4 xl:grid-cols-2">
        <Absences workspace={workspace} />
        <Incidents workspace={workspace} />
      </div>
      <Notifications items={workspace.notifications} />
      <Flags workspace={workspace} />
      <Briefs workspace={workspace} />
    </div>
  );
}
