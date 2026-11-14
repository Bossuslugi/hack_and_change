import { useEffect, useRef, useState, type ReactNode } from 'react';
import type { ChecklistItem, Workspace } from '../api/schemas';
import { formatMinutes, initials, plural, STATUS_LABEL } from '../lib/format';
import { groupByStage, stepMeta, STEP_PROMPTS, type StageGroup } from '../lib/onboarding';
import { useUi } from '../store/ui';
import { Icon } from './ui/icons';
import { Badge, Button, Card, CardTitle, Eyebrow, IconTile, StatusDot, cn, type Tone } from './ui/primitives';

const STATUS_TONE: Record<ChecklistItem['status'], Tone> = { pending: 'neutral', in_progress: 'warn', completed: 'ok' };
const SEGMENT: Record<ChecklistItem['status'], string> = {
  pending: 'bg-white/15',
  in_progress: 'bg-warn-500 glow-amber',
  completed: 'bg-ok-500 glow-green',
};

function StatusIcon({ status, justCompleted }: { status: ChecklistItem['status']; justCompleted?: boolean }) {
  if (status === 'completed') {
    return (
      <span
        className={cn(
          'grid h-5 w-5 shrink-0 place-items-center rounded-full bg-[#00C257] text-white transition-all duration-500 ease-out',
          justCompleted && 'animate-checkmark-pop scale-110 shadow-sm shadow-[#00C257]/50',
        )}
        aria-hidden="true"
      >
        <Icon name="check" className={cn('h-3 w-3', justCompleted && 'scale-110')} strokeWidth="2.4" />
      </span>
    );
  }
  if (status === 'in_progress') {
    return (
      <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full border-2 border-warn-500" aria-hidden="true">
        <span className="h-2 w-2 rounded-full bg-warn-500" />
      </span>
    );
  }
  return <span className="block h-5 w-5 shrink-0 rounded-full border-2 border-ink-400/50" aria-hidden="true" />;
}

function HeroMetric({ label, children, testId }: { label: string; children: ReactNode; testId?: string }) {
  return (
    <div className="min-w-0 rounded-card border border-white/10 bg-white/10 px-3 py-2 backdrop-blur-sm">
      <p className="truncate text-meta text-white/60">{label}</p>
      <p className="mt-1 whitespace-nowrap font-mono text-body font-semibold text-white" data-testid={testId}>
        {children}
      </p>
    </div>
  );
}

/** Тёмный якорь экрана: прогресс первых недель, по сегменту на шаг. */
function ProgressTile({ workspace }: { workspace: Workspace }) {
  const { checklist, progress, metrics } = workspace;
  return (
    <section className="hero-surface relative shrink-0 overflow-hidden rounded-panel p-5 text-white shadow-hero">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="font-mono text-meta uppercase tracking-wider text-white/50">Первые недели</p>
          <p className="mt-1 text-body font-semibold text-white">
            Выполнено {progress.completed} из {progress.total} ключевых {plural(progress.total, ['шага', 'шагов', 'шагов'])}
          </p>
        </div>
        <p className="shrink-0 text-display font-extrabold text-white">{progress.percent}%</p>
      </div>
      <div
        className="mt-5 flex h-2 gap-1"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={progress.total}
        aria-valuenow={progress.completed}
        aria-label="Прогресс чек-листа"
      >
        {checklist.map((item) => (
          <span key={item.id} title={item.title} className={cn('h-full flex-1 rounded-full transition-colors', SEGMENT[item.status])} />
        ))}
      </div>
      <div className="capsule-grid mt-5">
        <HeroMetric label="Сэкономлено" testId="minutes-saved">
          {formatMinutes(metrics.mentor_minutes_saved)}
        </HeroMetric>
        <HeroMetric label="Закрыто вопросов">{metrics.questions_answered}</HeroMetric>
        <HeroMetric label="В работе">{progress.in_progress}</HeroMetric>
      </div>
    </section>
  );
}

function FocusTile({ workspace }: { workspace: Workspace }) {
  const { next_step: next, sprint_task: task } = workspace;
  const cronResult = useUi((s) => s.cronResult);
  const stalled = task.status === 'in_progress' && task.commits === 0;
  return (
    <section className="tile-blue flex flex-col rounded-panel border p-4 shadow-lift">
      <div className="flex items-center gap-2">
        <IconTile name="spark" tone="accent" />
        <Eyebrow>Фокус дня</Eyebrow>
      </div>
      <p className="mt-3 text-body font-semibold text-ink-900">{next ? next.title : 'Все ключевые шаги закрыты'}</p>
      <div className="mt-auto flex flex-wrap items-center gap-2 pt-4">
        <span className="inline-flex min-h-6 items-center rounded-tag bg-ink-900 px-2 py-1 font-mono text-meta font-medium text-white" title={task.summary}>
          {task.key}
        </span>
        <Badge tone={stalled ? 'warn' : 'ok'} className="whitespace-nowrap">
          <Icon name="clock" className="h-3 w-3" />
          {stalled ? `${task.days_in_progress} ${plural(task.days_in_progress, ['день', 'дня', 'дней'])} без коммитов` : `коммитов: ${task.commits}`}
        </Badge>
      </div>
      {cronResult && (
        <p role="status" className="mt-3 text-meta text-ink-500">
          {cronResult.reason}
        </p>
      )}
    </section>
  );
}

function MentorTile({ workspace }: { workspace: Workspace }) {
  const { intern, metrics, checklist } = workspace;
  const oneOnOne = checklist.find((item) => item.id === 'first_one_on_one');
  const briefReady = metrics.briefs_prepared > 0;
  return (
    <section className="tile-violet flex flex-col rounded-panel border p-4 shadow-lift">
      <div className="flex items-center gap-2">
        <IconTile name="mentor" tone="ai" />
        <Eyebrow>Наставник</Eyebrow>
      </div>
      <div className="mt-3 flex items-center gap-3">
        <span className="relative shrink-0">
          <span className="block rounded-full bg-gradient-to-br from-ai-500 to-accent-500 p-0.5" aria-hidden="true">
            <span className="grid h-10 w-10 place-items-center rounded-full bg-white font-mono text-meta font-semibold text-ai-700">{initials(intern.mentor_name)}</span>
          </span>
          <span className="absolute -bottom-0.5 -right-0.5 flex rounded-full bg-white p-0.5">
            <StatusDot pulse />
          </span>
        </span>
        <div className="min-w-0">
          <p className="truncate text-body font-semibold text-ink-900">{intern.mentor_name}</p>
          <p className="text-meta text-ink-500">в сети</p>
        </div>
      </div>
      <div className="mt-auto flex flex-wrap items-center gap-2 pt-4">
        <Badge tone={briefReady ? 'ai' : 'neutral'} className="whitespace-nowrap">
          <Icon name="spark" className="h-3 w-3" />
          {briefReady ? 'Бриф к 1:1 готов' : 'Бриф к 1:1 не собран'}
        </Badge>
        {oneOnOne && <span className="whitespace-nowrap text-meta text-ink-400">1:1 · {STATUS_LABEL[oneOnOne.status].toLowerCase()}</span>}
      </div>
    </section>
  );
}

function Step({ item, isNext, onAsk }: { item: ChecklistItem; isNext: boolean; onAsk?: (text: string) => void }) {
  const prompt = STEP_PROMPTS[item.id];
  const done = item.status === 'completed';
  const prevStatusRef = useRef(item.status);
  const [justCompleted, setJustCompleted] = useState(false);

  useEffect(() => {
    if (prevStatusRef.current !== 'completed' && item.status === 'completed') {
      setJustCompleted(true);
      const timer = setTimeout(() => setJustCompleted(false), 3000);
      return () => clearTimeout(timer);
    }
    prevStatusRef.current = item.status;
  }, [item.status]);

  return (
    <li
      data-testid={`step-${item.id}`}
      data-status={item.status}
      data-just-completed={justCompleted ? 'true' : undefined}
      className={cn(
        'rounded-card border p-4 transition-all duration-500 ease-out hover:shadow-lift',
        justCompleted && 'border-ok-500/40 bg-ok-500/5 ring-2 ring-ok-500/30 shadow-lift',
        !justCompleted && isNext && 'border-accent-500/40 bg-tint-blue',
        !justCompleted && !isNext && 'border-line bg-white hover:border-ink-400/40',
      )}
    >
      <div className="flex items-start gap-3">
        <div className="pt-0.5">
          <StatusIcon status={item.status} justCompleted={justCompleted} />
        </div>
        <div className="min-w-0 flex-1">
          <p className={cn('text-body', done ? 'text-ink-500' : 'font-medium text-ink-900')} title={item.description}>
            {item.title}
          </p>
          {item.evidence && item.status !== 'pending' && <p className="mt-1 text-meta text-ink-400">{item.evidence}</p>}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2 pl-8">
        <div className="flex flex-wrap items-center gap-2">
          <Badge mono className="whitespace-nowrap bg-ink-900/5">
            {stepMeta(item).service}
          </Badge>
          <Badge tone={STATUS_TONE[item.status]} className="whitespace-nowrap">
            <span aria-label={`Статус: ${STATUS_LABEL[item.status]}`}>{STATUS_LABEL[item.status]}</span>
          </Badge>
          {isNext && <span className="whitespace-nowrap text-meta font-semibold text-accent-700">Следующий шаг</span>}
        </div>
        {prompt && onAsk && !done && (
          <Button size="sm" variant={isNext ? 'dark' : 'secondary'} className="group" onClick={() => onAsk(prompt)}>
            Запустить с AI-Buddy
            <Icon name="arrowRight" className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
          </Button>
        )}
      </div>
    </li>
  );
}

function Stage({ group, index, nextId, onAsk }: { group: StageGroup; index: number; nextId?: string; onAsk?: (text: string) => void }) {
  return (
    <li>
      <div className="flex items-center gap-2">
        <span className="inline-flex min-h-6 items-center whitespace-nowrap rounded-tag bg-ink-900 px-2 py-1 font-mono text-meta font-medium uppercase tracking-wider text-white">
          Этап {index + 1}
        </span>
        <h3 className="min-w-0 truncate text-body font-semibold text-ink-900">{group.title}</h3>
        <span className="h-px flex-1 bg-line" aria-hidden="true" />
        <span className="shrink-0 font-mono text-meta text-ink-400">
          {group.completed}/{group.items.length}
        </span>
      </div>
      <ol className="mt-3 space-y-2">
        {group.items.map((item) => (
          <Step key={item.id} item={item} isNext={item.id === nextId} onAsk={onAsk} />
        ))}
      </ol>
    </li>
  );
}

/** Левая зона: прогресс, фокус дня, наставник и чек-лист по этапам. */
export function AdaptationMap({ workspace, onAsk }: { workspace: Workspace; onAsk?: (text: string) => void }) {
  const groups = groupByStage(workspace.checklist);
  return (
    <aside aria-label="Карта адаптации" className="scroll-thin flex min-h-0 w-[42%] shrink-0 flex-col gap-4 overflow-y-auto pb-1 pr-1">
      <ProgressTile workspace={workspace} />
      <div className="tile-grid shrink-0">
        <FocusTile workspace={workspace} />
        <MentorTile workspace={workspace} />
      </div>
      <Card className="shrink-0 p-5">
        <CardTitle aside={<span className="truncate font-mono text-meta text-ink-400">{workspace.intern.name}</span>}>Чек-лист адаптации</CardTitle>
        <ol className="mt-5 space-y-5">
          {groups.map((group, index) => (
            <Stage key={group.id} group={group} index={index} nextId={workspace.next_step?.id} onAsk={onAsk} />
          ))}
        </ol>
      </Card>
      <p className="flex shrink-0 items-start gap-2 px-2 pb-2 text-meta text-ink-400">
        <Icon name="warning" className="mt-0.5 h-3 w-3 shrink-0" />
        Синтетические данные: сотрудники, инструкции и корпоративные сервисы — имитации. Инструкции не являются регламентами Авито.
      </p>
    </aside>
  );
}
