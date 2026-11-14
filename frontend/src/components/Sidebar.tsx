import { useState } from 'react';
import type { ChecklistItem, Workspace } from '../api/schemas';
import { useCron } from '../hooks/useBuddy';
import { formatMinutes, plural, STATUS_LABEL } from '../lib/format';
import { useUi } from '../store/ui';
import { Badge, Button, Card, CardTitle, cn } from './ui/primitives';

/** Запросы к Гоффи по шагам чек-листа (совпадают с подсказками агента). */
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
      <span className="grid h-5 w-5 shrink-0 animate-pop place-items-center rounded-full bg-ok-500 text-white" aria-hidden="true">
        <svg viewBox="0 0 16 16" className="h-3 w-3">
          <path d="m3.5 8.5 3 3 6-7" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </span>
    );
  }
  if (status === 'in_progress') {
    return (
      <span className="relative grid h-5 w-5 shrink-0 place-items-center rounded-full border-2 border-brand-500" aria-hidden="true">
        <span className="absolute h-2 w-2 animate-pulse-ring rounded-full bg-brand-500" />
        <span className="h-2 w-2 rounded-full bg-brand-500" />
      </span>
    );
  }
  return <span className="h-5 w-5 shrink-0 rounded-full border-2 border-ink-300" aria-hidden="true" />;
}

function ProgressBar({ workspace }: { workspace: Workspace }) {
  const { progress } = workspace;
  return (
    <>
      <div
        className="h-2 overflow-hidden rounded-full bg-paper-100"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={progress.total}
        aria-valuenow={progress.completed}
        aria-label="Прогресс чек-листа"
      >
        <div
          className="h-full rounded-full bg-gradient-to-r from-brand-500 via-accent-500 to-ok-500 transition-[width] duration-700 ease-out"
          style={{ width: `${progress.percent}%` }}
        />
      </div>
      <p className="mt-1.5 text-xs text-ink-500">
        Выполнено {progress.completed} из {progress.total} {plural(progress.total, ['шага', 'шагов', 'шагов'])}
        {progress.in_progress > 0 && ` · в работе ${progress.in_progress}`}
      </p>
    </>
  );
}

function Checklist({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  const { checklist, progress, next_step: next } = workspace;
  return (
    <Card className="p-4">
      <CardTitle aside={<span className="text-xs font-medium text-ink-500">{progress.percent}%</span>}>Чек-лист адаптации</CardTitle>
      <div className="mt-3">
        <ProgressBar workspace={workspace} />
      </div>

      <ol className="mt-3 space-y-1">
        {checklist.map((item) => {
          const isNext = next?.id === item.id;
          const prompt = STEP_PROMPTS[item.id];
          return (
            <li
              key={item.id}
              data-testid={`step-${item.id}`}
              data-status={item.status}
              className={cn(
                'rounded-xl px-2.5 py-2 transition-all duration-200',
                isNext ? 'bg-brand-50 ring-1 ring-brand-500/30' : 'hover:bg-paper-100',
              )}
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
                    {isNext && <span className="font-semibold text-brand-700">Следующий шаг</span>}
                  </p>
                  {item.evidence && item.status !== 'pending' && <p className="mt-1 text-[11px] text-ink-500">{item.evidence}</p>}
                  {isNext && prompt && onAsk && (
                    <Button size="sm" variant="primary" className="mt-2" onClick={() => onAsk(prompt)}>
                      Спросить Гоффи
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
        <div className="mt-3 border-t border-paper-200 pt-3">
          <Button className="w-full" onClick={() => cron.mutate()} loading={cron.isPending}>
            Запустить ИИ-крон
          </Button>
          <p className="mt-1.5 text-[11px] leading-snug text-ink-400">
            Имитация фонового крона: проверит задачу, напишет стажёру и подготовит наставнику бриф к 1:1.
          </p>
          {cronResult && (
            <p role="status" className="mt-2 rounded-lg bg-paper-100 px-2.5 py-2 text-[11px] leading-snug text-ink-700">
              {cronResult.reason}
            </p>
          )}
        </div>
      )}
    </Card>
  );
}

function Stat({ value, label, tone = 'text-ink-900' }: { value: string | number; label: string; tone?: string }) {
  return (
    <div className="rounded-xl bg-paper-100 px-2.5 py-2 transition-transform duration-200 hover:-translate-y-0.5">
      <p className={cn('text-lg font-semibold leading-none', tone)}>{value}</p>
      <p className="mt-1 text-[11px] leading-tight text-ink-500">{label}</p>
    </div>
  );
}

/** Кольцевой индикатор «показателя самостоятельности». */
function Ring({ percent, empty }: { percent: number; empty: boolean }) {
  const r = 26;
  const circumference = 2 * Math.PI * r;
  return (
    <div className="relative h-[72px] w-[72px] shrink-0">
      <svg viewBox="0 0 64 64" className="h-full w-full -rotate-90" aria-hidden="true">
        <circle cx="32" cy="32" r={r} fill="none" strokeWidth="6" className="stroke-paper-200" />
        <circle
          cx="32"
          cy="32"
          r={r}
          fill="none"
          strokeWidth="6"
          strokeLinecap="round"
          className="stroke-ok-500 transition-[stroke-dashoffset] duration-700 ease-out"
          strokeDasharray={circumference}
          strokeDashoffset={circumference * (1 - percent / 100)}
        />
      </svg>
      <span className="absolute inset-0 grid place-items-center text-sm font-semibold text-ink-900" data-testid="deflection-rate">
        {empty ? '—' : `${percent}%`}
      </span>
    </div>
  );
}

/** Статистика стажёра: прогресс, самостоятельность, сэкономленное время, доступы и материалы. */
function Statistics({ workspace }: { workspace: Workspace }) {
  const m = workspace.metrics;
  const asked = m.questions_answered + m.unanswered_questions;
  return (
    <>
      <Card className="p-4">
        <CardTitle aside={<span className="text-xs font-medium text-ink-500">{workspace.progress.percent}%</span>}>Прогресс адаптации</CardTitle>
        <div className="mt-3">
          <ProgressBar workspace={workspace} />
        </div>
      </Card>

      <Card className="p-4">
        <CardTitle>Показатель самостоятельности</CardTitle>
        <div className="mt-3 flex items-center gap-3">
          <Ring percent={m.deflection_rate} empty={asked === 0} />
          <p className="text-xs leading-relaxed text-ink-500">
            {asked === 0
              ? 'Пока нет вопросов. Спроси Гоффи — здесь появится доля вопросов, закрытых без наставника.'
              : `${m.questions_answered} из ${asked} ${plural(asked, ['вопроса', 'вопросов', 'вопросов'])} Гоффи закрыл без отвлечения наставника (Deflection Rate).`}
          </p>
        </div>
      </Card>

      <Card className="p-4">
        <CardTitle>Сэкономлено времени наставника</CardTitle>
        <p className="mt-2 bg-gradient-to-r from-brand-500 to-accent-500 bg-clip-text text-3xl font-semibold text-transparent" data-testid="minutes-saved">
          {formatMinutes(m.mentor_minutes_saved)}
        </p>
        <p className="text-xs text-ink-500">
          ≈{m.saving_percent}% от {m.mentor_hours_baseline_min}–{m.mentor_hours_baseline_max} ч наставника за программу
        </p>
        <div className="mt-3 grid grid-cols-2 gap-2">
          <Stat value={m.access_grants} label="доступов получено" tone="text-brand-700" />
          <Stat value={`${m.docs_read}/${m.docs_total}`} label="материалов изучено" tone="text-accent-700" />
          <Stat value={m.actions_automated} label="действий через сервисы" />
          <Stat value={`${m.tasks_done}/${m.tasks_total}`} label="событий календаря закрыто" />
          <Stat value={m.rbac_blocks} label="запросов остановила защита" />
          <Stat value={m.briefs_prepared} label="брифов к 1:1" />
        </div>
        {m.unanswered_questions > 0 && <p className="mt-3 text-xs text-ink-700">Вопросов передано наставнику: {m.unanswered_questions}</p>}
        <p className="mt-2 text-[11px] text-ink-400">Оценка-гипотеза по весам из спецификации; способ проверки — в README.</p>
      </Card>
    </>
  );
}

type Tab = 'onboarding' | 'stats';

export function Sidebar({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  const [tab, setTab] = useState<Tab>('onboarding');
  const tabs: Array<{ id: Tab; label: string }> = [
    { id: 'onboarding', label: 'Адаптация' },
    { id: 'stats', label: 'Статистика' },
  ];
  return (
    <aside className="flex flex-col gap-4" aria-label="Адаптация стажёра">
      <div role="tablist" aria-label="Разделы левой панели" className="bento flex gap-1 p-1">
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              'flex-1 rounded-2xl px-3 py-1.5 text-sm font-medium transition-all duration-200',
              'focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500',
              tab === t.id ? 'bg-brand-500 text-white shadow-glow' : 'text-ink-500 hover:text-ink-900',
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === 'onboarding' ? (
        <>
          <Checklist workspace={workspace} onAsk={onAsk} />
          <TaskCard workspace={workspace} />
        </>
      ) : (
        <Statistics workspace={workspace} />
      )}
    </aside>
  );
}
