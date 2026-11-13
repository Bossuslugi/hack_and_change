import type { ChecklistItem, Workspace } from '../api/schemas';
import { useCron } from '../hooks/useBuddy';
import { formatMinutes, plural, STATUS_LABEL } from '../lib/format';
import { useUi } from '../store/ui';
import { Badge, Button, Card, CardTitle, cn } from './ui/primitives';

/** Запросы к AI-Buddy по шагам чек-листа (совпадают с подсказками агента). */
export const STEP_PROMPTS: Record<string, string> = {
  access_repo: 'Дай доступ к репозиторию avito-autoteka-api и тестовому стенду',
  team_processes: 'Как в нашей команде катить задачу под фича-флагом и проходить ревью?',
  first_task: 'Я застрял на первой задаче, с чего начать?',
  absence_rules: 'Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?',
  first_one_on_one: 'Как проходят встречи 1:1?',
};

function StatusIcon({ status }: { status: ChecklistItem['status'] }) {
  if (status === 'completed') {
    return (
      <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-ok-500 text-white" aria-hidden="true">
        <svg viewBox="0 0 16 16" className="h-3 w-3">
          <path d="m3.5 8.5 3 3 6-7" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </span>
    );
  }
  if (status === 'in_progress') {
    return (
      <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full border-2 border-brand-500" aria-hidden="true">
        <span className="h-2 w-2 rounded-full bg-brand-500" />
      </span>
    );
  }
  return <span className="h-5 w-5 shrink-0 rounded-full border-2 border-ink-300" aria-hidden="true" />;
}

function Checklist({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  const { checklist, progress, next_step: next } = workspace;
  return (
    <Card className="p-4">
      <CardTitle aside={<span className="text-xs font-medium text-ink-500">{progress.percent}%</span>}>Чек-лист адаптации</CardTitle>
      <div
        className="mt-3 h-2 overflow-hidden rounded-full bg-paper-100"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={progress.total}
        aria-valuenow={progress.completed}
        aria-label="Прогресс чек-листа"
      >
        <div className="h-full rounded-full bg-ok-500 transition-[width] duration-500" style={{ width: `${progress.percent}%` }} />
      </div>
      <p className="mt-1.5 text-xs text-ink-500">
        Выполнено {progress.completed} из {progress.total} {plural(progress.total, ['шага', 'шагов', 'шагов'])}
        {progress.in_progress > 0 && ` · в работе ${progress.in_progress}`}
      </p>

      <ol className="mt-3 space-y-1">
        {checklist.map((item) => {
          const isNext = next?.id === item.id;
          const prompt = STEP_PROMPTS[item.id];
          return (
            <li
              key={item.id}
              data-testid={`step-${item.id}`}
              data-status={item.status}
              className={cn('rounded-xl px-2.5 py-2 transition-colors', isNext ? 'bg-brand-50 ring-1 ring-brand-100' : 'hover:bg-paper-50')}
            >
              <div className="flex items-start gap-2.5">
                <div className="mt-0.5">
                  <StatusIcon status={item.status} />
                </div>
                <div className="min-w-0 flex-1">
                  <p className={cn('text-sm leading-snug', item.status === 'completed' ? 'text-ink-500' : 'font-medium text-ink-900')}>
                    <span className="mr-1 text-ink-400">{item.position}.</span>
                    {item.title}
                  </p>
                  <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-[11px] text-ink-400">
                    <span>Неделя {item.week}</span>
                    <span aria-label={`Статус: ${STATUS_LABEL[item.status]}`}>{STATUS_LABEL[item.status]}</span>
                    {isNext && <span className="font-semibold text-brand-600">Следующий шаг</span>}
                  </p>
                  {item.evidence && item.status !== 'pending' && <p className="mt-1 text-[11px] text-ink-500">{item.evidence}</p>}
                  {isNext && prompt && onAsk && (
                    <Button size="sm" variant="primary" className="mt-2" onClick={() => onAsk(prompt)}>
                      Спросить AI-Buddy
                    </Button>
                  )}
                </div>
              </div>
            </li>
          );
        })}
      </ol>
    </Card>
  );
}

function TaskCard({ workspace }: { workspace: Workspace }) {
  const task = workspace.sprint_task;
  const cron = useCron();
  const cronResult = useUi((s) => s.cronResult);
  const stalled = task.status === 'in_progress' && task.commits === 0;

  return (
    <Card className="p-4">
      <CardTitle aside={<Badge tone={stalled ? 'warn' : 'ok'}>{stalled ? 'Без коммитов' : 'Есть прогресс'}</Badge>}>Задача спринта</CardTitle>
      <p className="mt-2 text-sm font-medium text-ink-900">
        <span className="font-mono text-xs text-ink-500">{task.key}</span> · {task.title}
      </p>
      <p className="mt-1 text-xs leading-relaxed text-ink-500">{task.summary}</p>
      <p className="mt-2 text-xs text-ink-700">
        В работе {task.days_in_progress} {plural(task.days_in_progress, ['день', 'дня', 'дней'])} · коммитов: {task.commits}
      </p>

      {workspace.permissions.cron && (
        <div className="mt-3 border-t border-paper-100 pt-3">
          <Button className="w-full" onClick={() => cron.mutate()} loading={cron.isPending}>
            Запустить ИИ-крон
          </Button>
          <p className="mt-1.5 text-[11px] leading-snug text-ink-400">
            Имитация фонового крона: проверит задачу, напишет стажёру и подготовит наставнику бриф к 1:1.
          </p>
          {cronResult && (
            <p role="status" className="mt-2 rounded-lg bg-paper-50 px-2.5 py-2 text-[11px] leading-snug text-ink-700">
              {cronResult.reason}
            </p>
          )}
        </div>
      )}
    </Card>
  );
}

function Stat({ value, label }: { value: number; label: string }) {
  return (
    <div className="rounded-xl bg-paper-50 px-2.5 py-2">
      <p className="text-lg font-semibold leading-none text-ink-900">{value}</p>
      <p className="mt-1 text-[11px] leading-tight text-ink-500">{label}</p>
    </div>
  );
}

function MetricsCard({ workspace }: { workspace: Workspace }) {
  const m = workspace.metrics;
  return (
    <Card className="p-4">
      <CardTitle>Эффект для наставника</CardTitle>
      <p className="mt-2 text-2xl font-semibold text-ink-900" data-testid="minutes-saved">
        {formatMinutes(m.mentor_minutes_saved)}
      </p>
      <p className="text-xs text-ink-500">
        сэкономлено · ≈{m.saving_percent}% от {m.mentor_hours_baseline_min}–{m.mentor_hours_baseline_max} ч наставника за программу
      </p>
      <div className="mt-3 grid grid-cols-2 gap-2">
        <Stat value={m.questions_answered} label="вопросов закрыто без наставника" />
        <Stat value={m.actions_automated} label="действий через сервисы" />
        <Stat value={m.rbac_blocks} label="блокировок RBAC" />
        <Stat value={m.briefs_prepared} label="брифов к 1:1" />
      </div>
      <p className="mt-3 text-xs text-ink-700">
        Инструкций изучено: {m.docs_read} из {m.docs_total}
        {m.unanswered_questions > 0 && ` · вопросов к наставнику: ${m.unanswered_questions}`}
      </p>
      <p className="mt-1 text-[11px] text-ink-400">Оценка-гипотеза по весам из спецификации; способ проверки — в README.</p>
    </Card>
  );
}

export function Sidebar({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  return (
    <aside className="flex flex-col gap-4" aria-label="Адаптация стажёра">
      <Checklist workspace={workspace} onAsk={onAsk} />
      <TaskCard workspace={workspace} />
      <MetricsCard workspace={workspace} />
    </aside>
  );
}
