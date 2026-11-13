import { useEffect, useRef, useState } from 'react';
import type { ChecklistItem, Workspace } from '../api/schemas';
import { formatMinutes, initials, plural, STATUS_LABEL } from '../lib/format';
import { groupByStage, stepMeta, STEP_PROMPTS, type StageGroup } from '../lib/onboarding';
import { useUi } from '../store/ui';
import { Icon } from './ui/icons';
import { Badge, Button, Card, CardTitle, Eyebrow, cn, type Tone } from './ui/primitives';

const STATUS_TONE: Record<ChecklistItem['status'], Tone> = { pending: 'neutral', in_progress: 'warn', completed: 'ok' };
const SEGMENT: Record<ChecklistItem['status'], string> = { pending: 'bg-ink-900/10', in_progress: 'bg-warn-500', completed: 'bg-ok-500' };

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

/** Прогресс первых недель: по сегменту на шаг, цвет сегмента — статус шага. */
function ProgressTile({ workspace }: { workspace: Workspace }) {
  const { checklist, progress, metrics } = workspace;
  return (
    <Card className="col-span-2 p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <Eyebrow>Первые недели</Eyebrow>
          <p className="mt-1 text-body font-semibold text-ink-900">
            Выполнено {progress.completed} из {progress.total} ключевых {plural(progress.total, ['шага', 'шагов', 'шагов'])}
          </p>
        </div>
        <p className="text-title font-semibold text-ink-900">{progress.percent}%</p>
      </div>
      <div
        className="mt-4 flex h-2 gap-1"
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
      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-meta text-ink-500">
        <span>
          Наставнику сэкономлено{' '}
          <span className="font-mono font-medium text-ink-900" data-testid="minutes-saved">
            {formatMinutes(metrics.mentor_minutes_saved)}
          </span>
        </span>
        <span>
          Вопросов закрыто без наставника <span className="font-mono font-medium text-ink-900">{metrics.questions_answered}</span>
        </span>
        {progress.in_progress > 0 && (
          <span>
            В работе <span className="font-mono font-medium text-ink-900">{progress.in_progress}</span>
          </span>
        )}
      </div>
    </Card>
  );
}

function FocusTile({ workspace }: { workspace: Workspace }) {
  const { next_step: next, sprint_task: task } = workspace;
  const cronResult = useUi((s) => s.cronResult);
  const stalled = task.status === 'in_progress' && task.commits === 0;
  return (
    <Card className="flex flex-col p-4">
      <Eyebrow>Фокус дня</Eyebrow>
      <p className="mt-1 text-body font-semibold text-ink-900">{next ? next.title : 'Все ключевые шаги закрыты'}</p>
      <div className="mt-auto pt-4">
        <div className="flex flex-wrap items-center gap-2">
          <Badge mono title={task.summary}>
            {task.key}
          </Badge>
          <Badge tone={stalled ? 'warn' : 'ok'}>
            <Icon name="clock" className="h-3 w-3" />
            {stalled
              ? `${task.days_in_progress} ${plural(task.days_in_progress, ['день', 'дня', 'дней'])} без коммитов`
              : `коммитов: ${task.commits}`}
          </Badge>
        </div>
        {cronResult && (
          <p role="status" className="mt-2 text-meta text-ink-500">
            {cronResult.reason}
          </p>
        )}
      </div>
    </Card>
  );
}

function MentorTile({ workspace }: { workspace: Workspace }) {
  const { intern, metrics, checklist } = workspace;
  const oneOnOne = checklist.find((item) => item.id === 'first_one_on_one');
  const briefReady = metrics.briefs_prepared > 0;
  return (
    <Card className="flex flex-col p-4">
      <Eyebrow>Наставник</Eyebrow>
      <div className="mt-2 flex items-center gap-2">
        <span className="relative grid h-8 w-8 shrink-0 place-items-center rounded-full bg-ai-50 font-mono text-meta font-medium text-ai-700" aria-hidden="true">
          {initials(intern.mentor_name)}
          <span className="absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full bg-ok-500 ring-2 ring-white" />
        </span>
        <div className="min-w-0">
          <p className="truncate text-body font-semibold text-ink-900">{intern.mentor_name}</p>
          <p className="text-meta text-ink-500">в сети</p>
        </div>
      </div>
      <div className="mt-auto flex flex-wrap items-center gap-2 pt-4">
        <Badge tone={briefReady ? 'ai' : 'neutral'}>
          <Icon name="spark" className="h-3 w-3" />
          {briefReady ? 'Бриф к 1:1 готов' : 'Бриф к 1:1 не собран'}
        </Badge>
        {oneOnOne && <span className="text-meta text-ink-400">1:1 · {STATUS_LABEL[oneOnOne.status].toLowerCase()}</span>}
      </div>
    </Card>
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
        'flex items-start gap-2 rounded-card p-2 transition-all duration-500 ease-out',
        justCompleted && 'ring-2 ring-[#00C257]/40 bg-[#E8F9EF]/40 shadow-sm',
        !justCompleted && isNext && 'bg-accent-50',
        !justCompleted && !isNext && 'hover:bg-canvas',
      )}
    >
      <div className="pt-0.5">
        <StatusIcon status={item.status} justCompleted={justCompleted} />
      </div>
      <div className="min-w-0 flex-1">
        <p className={cn('text-body', done ? 'text-ink-500' : 'font-medium text-ink-900')} title={item.description}>
          {item.title}
        </p>
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <Badge mono className="bg-ink-900/5">
            {stepMeta(item).service}
          </Badge>
          <Badge tone={STATUS_TONE[item.status]}>
            <span aria-label={`Статус: ${STATUS_LABEL[item.status]}`}>{STATUS_LABEL[item.status]}</span>
          </Badge>
          {isNext && <span className="text-meta font-medium text-accent-700">Следующий шаг</span>}
        </div>
        {item.evidence && item.status !== 'pending' && <p className="mt-1 text-meta text-ink-400">{item.evidence}</p>}
      </div>
      {prompt && onAsk && !done && (
        <Button size="sm" variant={isNext ? 'primary' : 'secondary'} className="mt-0.5" onClick={() => onAsk(prompt)}>
          Запустить с AI-Buddy
          <Icon name="arrowRight" className="h-3 w-3" />
        </Button>
      )}
    </li>
  );
}

function Stage({ group, index, nextId, onAsk }: { group: StageGroup; index: number; nextId?: string; onAsk?: (text: string) => void }) {
  return (
    <li>
      <div className="flex items-center gap-2 px-2">
        <span className="font-mono text-meta uppercase tracking-wider text-ink-400">Этап {index + 1}</span>
        <h3 className="text-meta font-semibold text-ink-900">{group.title}</h3>
        <span className="h-px flex-1 bg-line" aria-hidden="true" />
        <span className="font-mono text-meta text-ink-400">
          {group.completed}/{group.items.length}
        </span>
      </div>
      <ol className="mt-1 space-y-1">
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
    <aside aria-label="Карта адаптации" className="scroll-thin flex min-h-0 w-[42%] shrink-0 flex-col gap-2 overflow-y-auto">
      <div className="grid shrink-0 grid-cols-2 gap-2">
        <ProgressTile workspace={workspace} />
        <FocusTile workspace={workspace} />
        <MentorTile workspace={workspace} />
      </div>
      <Card className="shrink-0 p-4">
        <CardTitle aside={<span className="font-mono text-meta text-ink-400">{workspace.intern.name}</span>}>Чек-лист адаптации</CardTitle>
        <ol className="mt-4 space-y-4">
          {groups.map((group, index) => (
            <Stage key={group.id} group={group} index={index} nextId={workspace.next_step?.id} onAsk={onAsk} />
          ))}
        </ol>
      </Card>
      <p className="flex shrink-0 items-start gap-2 px-2 pb-2 text-meta text-ink-400">
        <Icon name="warning" className="mt-0.5 h-3 w-3" />
        Синтетические данные: сотрудники, инструкции и корпоративные сервисы — имитации. Инструкции не являются регламентами Авито.
      </p>
    </aside>
  );
}
