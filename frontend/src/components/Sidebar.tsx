import { useState } from 'react';
import type { ChecklistItem, Workspace } from '../api/schemas';
import { useCron } from '../hooks/useBuddy';
import { AVITO_BENCHMARKS, AVITO_SERVICES, MENTOR_LOAD, STEP_QUIZ, STEP_SERVICES, okrProgress, okrsFor, stepSpec, type Benchmark } from '../lib/avito';
import { formatMinutes, initials, plural, STATUS_LABEL } from '../lib/format';
import { useUi } from '../store/ui';
import { Planner } from './Planner';
import { Badge, Button, Card, CardTitle, OnlineDot, cn } from './ui/primitives';

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
  return <span className="block h-5 w-5 shrink-0 rounded-full border-2 border-ink-300" aria-hidden="true" />;
}

/** Компактная полоса прогресса над вкладками: процент, шаги и следующий шаг в одну строку. */
function HeroBar({ workspace }: { workspace: Workspace }) {
  const { progress, next_step: next, intern } = workspace;
  return (
    <section className="bento flex shrink-0 items-center gap-4 overflow-hidden px-4 py-3" aria-label="Прогресс стажёра">
      <p className="shrink-0 bg-gradient-to-r from-brand-500 to-accent-500 bg-clip-text text-3xl font-bold leading-none text-transparent">{progress.percent}%</p>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5" aria-label="Контекст стажёра">
            <p className="mr-1 text-sm font-semibold text-ink-900">{intern.name}</p>
            <Badge tone="brand">Команда {intern.team}</Badge>
            <Badge tone="brand">{intern.stack}</Badge>
            <Badge tone="accent">{intern.stage}</Badge>
          </div>
          <p className="shrink-0 text-xs text-ink-500">
            Выполнено {progress.completed} из {progress.total} {plural(progress.total, ['шага', 'шагов', 'шагов'])}
            {progress.in_progress > 0 && ` · в работе ${progress.in_progress}`}
          </p>
        </div>
        <div
          className="mt-2 h-2 overflow-hidden rounded-full bg-paper-100"
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
        <p className="mt-1.5 truncate text-xs text-ink-500">{next ? `Дальше: ${next.title}` : 'Все шаги адаптации закрыты'}</p>
      </div>
    </section>
  );
}

/** Мини-квиз от Гоффи: один вопрос, ответ в один клик. Проверяет себя сам стажёр, шаг закрывает агент или наставник. */
function StepQuizCard({ stepId }: { stepId: string }) {
  const quiz = STEP_QUIZ[stepId];
  const [open, setOpen] = useState(false);
  const [answer, setAnswer] = useState<number | null>(null);
  if (!quiz) return null;
  const correct = answer === quiz.correct;
  return (
    <div className="mt-2">
      <Button size="sm" variant="ghost" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
        Мини-квиз
      </Button>
      {open && (
        <div className="glass-inner mt-2 p-3" role="group" aria-label={`Мини-квиз: ${quiz.question}`}>
          <p className="text-xs font-medium text-ink-900">{quiz.question}</p>
          <div className="mt-2 flex flex-col gap-1.5">
            {quiz.options.map((option, i) => (
              <button
                key={option}
                type="button"
                aria-pressed={answer === i}
                onClick={() => setAnswer(i)}
                className={cn(
                  'rounded-xl border px-3 py-1.5 text-left text-xs transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500',
                  answer !== i && 'border-paper-200 bg-paper-0 text-ink-700 hover:border-brand-500/40',
                  answer === i && i === quiz.correct && 'border-ok-500/40 bg-ok-50 text-ok-700',
                  answer === i && i !== quiz.correct && 'border-danger-500/40 bg-danger-50 text-danger-700',
                )}
              >
                {option}
              </button>
            ))}
          </div>
          {answer !== null && (
            <p role="status" className={cn('mt-2 text-[11px] leading-snug', correct ? 'text-ok-700' : 'text-danger-700')}>
              {correct ? `Верно. ${quiz.explanation}` : 'Не совсем. Попробуй ещё раз или спроси Гоффи.'}
            </p>
          )}
          <p className="mt-1.5 text-[11px] text-ink-400">Самопроверка. Шаг закроет Гоффи после разбора темы или наставник на 1:1.</p>
        </div>
      )}
    </div>
  );
}

/** Паспорт активного шага: ресурс, метрики, условие выкатки и согласующий. */
function StepPassport({ item, workspace }: { item: ChecklistItem; workspace: Workspace }) {
  const { intern, sprint_task: task } = workspace;
  const rows = stepSpec({
    stepId: item.id,
    week: item.week,
    description: item.description,
    stack: intern.stack,
    repository: intern.repository,
    mentorName: intern.mentor_name,
    taskKey: task.key,
  });
  return (
    <dl
      className="glass-inner mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 p-3 text-[11px]"
      aria-label={item.id === 'access_repo' ? 'Состав заявки на доступ' : 'Паспорт шага'}
    >
      {rows.map((row) => (
        <div key={row.label} className="contents">
          <dt className="text-ink-500">{row.label}</dt>
          <dd className="text-ink-900">{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function Checklist({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  const { checklist, progress, next_step: next } = workspace;
  return (
    <Card className="overflow-hidden p-4">
      <CardTitle aside={<Badge tone="brand">{progress.completed}/{progress.total}</Badge>}>Чек-лист адаптации</CardTitle>

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
                'rounded-2xl px-2.5 py-2 transition-all duration-200',
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
                    {(STEP_SERVICES[item.id] ?? []).map((service) => (
                      <Badge key={service}>{service}</Badge>
                    ))}
                  </p>
                  {item.evidence && item.status !== 'pending' && <p className="mt-1 text-[11px] text-ink-500">{item.evidence}</p>}
                  {isNext && <StepPassport item={item} workspace={workspace} />}
                  {isNext && prompt && onAsk && (
                    <Button size="sm" variant="primary" className="mt-2" onClick={() => onAsk(prompt)}>
                      {item.id === 'access_repo' ? 'Запросить через Гоффи' : 'Спросить Гоффи'}
                    </Button>
                  )}
                  {item.status !== 'completed' && onAsk && <StepQuizCard stepId={item.id} />}
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
    <Card className="overflow-hidden p-4">
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
            <p role="status" className="mt-2 rounded-xl bg-paper-100 px-2.5 py-2 text-[11px] leading-snug text-ink-700">
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
    <div className="rounded-2xl bg-paper-100 px-2.5 py-2 transition-transform duration-200 hover:-translate-y-0.5">
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
      <Card className="overflow-hidden p-4">
        <CardTitle aside={<span className="text-xs font-medium text-ink-500">{workspace.progress.percent}%</span>}>Прогресс адаптации</CardTitle>
        <div className="mt-3 grid grid-cols-3 gap-2">
          <Stat value={workspace.progress.completed} label="шагов закрыто" tone="text-ok-700" />
          <Stat value={workspace.progress.in_progress} label="в работе" tone="text-brand-700" />
          <Stat value={workspace.progress.total - workspace.progress.completed - workspace.progress.in_progress} label="впереди" />
        </div>
      </Card>

      <Card className="overflow-hidden p-4">
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

      <Card className="overflow-hidden p-4">
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

      <AvitoBenchmarks />
      <MentorLoad />
    </>
  );
}

const BENCHMARK_TONE: Record<Benchmark['tone'], string> = {
  brand: 'text-brand-700',
  accent: 'text-accent-700',
  ok: 'text-ok-700',
  warn: 'text-warn-700',
};

/** Ориентиры процесса онбординга из Q&A с организаторами. */
export function AvitoBenchmarks() {
  return (
    <Card className="overflow-hidden p-4">
      <CardTitle aside={<Badge tone="accent">Q&A организаторов</Badge>}>Бенчмарки онбординга Авито</CardTitle>
      <ul className="mt-3 grid gap-2 sm:grid-cols-2" aria-label="Бенчмарки онбординга">
        {AVITO_BENCHMARKS.map((b) => (
          <li key={b.id} data-testid="benchmark" className={cn('glass-inner p-3', b.id === 'checklist_csat' && 'sm:col-span-2')}>
            <p className="text-[11px] text-ink-500">{b.label}</p>
            <p className={cn('mt-0.5 text-xl font-semibold leading-tight', BENCHMARK_TONE[b.tone])}>{b.value}</p>
            <p className="mt-1 text-[11px] leading-snug text-ink-500">{b.note}</p>
          </li>
        ))}
      </ul>
      <p className="mt-2 text-[11px] text-ink-400">Справочные цифры процесса. Метрики прототипа выше считает бэкенд, прогнозы — гипотезы.</p>
    </Card>
  );
}

/** Сколько времени наставник тратит на стажёра по фазам программы. */
export function MentorLoad() {
  return (
    <Card className="overflow-hidden p-4">
      <CardTitle>Нагрузка наставника</CardTitle>
      <ul className="mt-3 space-y-3">
        {MENTOR_LOAD.phases.map((phase) => (
          <li key={phase.id}>
            <div className="flex items-baseline justify-between gap-3 text-xs">
              <span className="font-medium text-ink-900">{phase.label}</span>
              <span className="font-mono text-ink-700">{phase.hours}</span>
            </div>
            <div className="mt-1 h-2 overflow-hidden rounded-full bg-paper-100" aria-hidden="true">
              <div className="h-full rounded-full bg-gradient-to-r from-brand-500 to-accent-500" style={{ width: `${phase.share}%` }} />
            </div>
            <p className="mt-1 text-[11px] text-ink-500">{phase.note}</p>
          </li>
        ))}
      </ul>
      <div className="mt-3 grid grid-cols-2 gap-2">
        {MENTOR_LOAD.totals.map((t) => (
          <Stat key={t.label} value={t.value} label={t.label} />
        ))}
      </div>
    </Card>
  );
}

/** Цели на адаптацию: согласованы с руководителем, прогресс считается по связанным шагам чек-листа. */
function Okrs({ workspace }: { workspace: Workspace }) {
  const { intern, checklist } = workspace;
  return (
    <Card className="overflow-hidden p-4">
      <CardTitle aside={<Badge tone="accent">3 цели</Badge>}>Цели на адаптацию и OKR</CardTitle>
      <p className="mt-0.5 text-[11px] text-ink-400">Согласовано с руководителем: {intern.team_lead_name}</p>
      <ol className="mt-3 grid gap-2 xl:grid-cols-3" aria-label="Цели на адаптацию">
        {okrsFor(intern.stack).map((okr) => {
          const percent = okrProgress(okr, checklist);
          return (
            <li key={okr.text} data-testid="okr" className="glass-inner flex flex-col gap-2 p-3">
              <p className="flex-1 text-xs font-medium leading-snug text-ink-900">{okr.text}</p>
              <div className="flex items-center gap-2">
                <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-paper-100" aria-hidden="true">
                  <span className="block h-full rounded-full bg-gradient-to-r from-brand-500 to-ok-500" style={{ width: `${percent}%` }} />
                </span>
                <span className="font-mono text-[11px] text-ink-500">{percent}%</span>
              </div>
            </li>
          );
        })}
      </ol>
      <ul className="mt-3 flex flex-wrap gap-1.5" aria-label="Сервисы команды">
        {AVITO_SERVICES.map((service) => (
          <li key={service.name}>
            <Badge title={service.purpose}>
              {service.name}
              <span className="font-normal text-ink-500">· {service.purpose}</span>
            </Badge>
          </li>
        ))}
      </ul>
    </Card>
  );
}

function MentorCard({ workspace }: { workspace: Workspace }) {
  const { intern, metrics } = workspace;
  return (
    <Card className="flex items-center gap-3 overflow-hidden p-4">
      <span className="rounded-full bg-gradient-to-br from-brand-500 to-accent-500 p-0.5" aria-hidden="true">
        <span className="grid h-10 w-10 place-items-center rounded-full bg-paper-0 text-xs font-semibold text-accent-700">{initials(intern.mentor_name)}</span>
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-[11px] text-ink-500">Наставник</p>
        <p className="flex items-center gap-2 truncate text-sm font-semibold text-ink-900">
          {intern.mentor_name} <OnlineDot />
        </p>
      </div>
      <Badge tone={metrics.briefs_prepared > 0 ? 'accent' : 'neutral'}>{metrics.briefs_prepared > 0 ? 'Бриф к 1:1 готов' : 'Бриф к 1:1 не собран'}</Badge>
    </Card>
  );
}

type Tab = 'onboarding' | 'calendar' | 'stats';

const TABS: Array<{ id: Tab; name: string; label: string; icon: string }> = [
  { id: 'onboarding', name: 'Адаптация', label: 'Адаптация и OKR', icon: '🗺' },
  { id: 'calendar', name: 'Календарь', label: 'Календарь и задачи', icon: '📅' },
  { id: 'stats', name: 'Статистика', label: 'Статистика и метрики', icon: '📊' },
];

/** Левая колонка: полоса прогресса, переключатель разделов и содержимое выбранного раздела на всю высоту. */
export function Sidebar({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  const [tab, setTab] = useState<Tab>('onboarding');
  const todo = workspace.planner.filter((item) => item.status !== 'done').length;
  return (
    <aside className="flex min-h-0 flex-col gap-3 lg:h-full" aria-label="Адаптация стажёра">
      <HeroBar workspace={workspace} />
      <div role="tablist" aria-label="Разделы левой панели" className="bento flex shrink-0 gap-1 !rounded-2xl p-1">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-label={t.name}
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 rounded-xl px-3 py-1.5 text-sm font-medium transition-all duration-200',
              'focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500',
              tab === t.id ? 'bg-brand-500 text-white shadow-glow' : 'text-ink-500 hover:text-ink-900',
            )}
          >
            <span aria-hidden="true">{t.icon}</span>
            <span className="truncate">{t.label}</span>
            {t.id === 'calendar' && todo > 0 && (
              <span className={cn('rounded-full px-1.5 text-[11px]', tab === t.id ? 'bg-white/20' : 'bg-paper-100 text-ink-700')}>{todo}</span>
            )}
          </button>
        ))}
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pr-1 stack-scroll">
        {tab === 'onboarding' && (
          <>
            <Okrs workspace={workspace} />
            <Checklist workspace={workspace} onAsk={onAsk} />
            <MentorCard workspace={workspace} />
            <TaskCard workspace={workspace} />
          </>
        )}
        {/* Календарь смонтирован всегда: напоминания и открытые карточки событий не теряются при смене вкладки. */}
        <div className={tab === 'calendar' ? 'flex flex-col gap-3' : 'hidden'} aria-label="Календарь и напоминания">
          <Planner workspace={workspace} />
        </div>
        {tab === 'stats' && <Statistics workspace={workspace} />}
      </div>
    </aside>
  );
}
