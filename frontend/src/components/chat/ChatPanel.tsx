import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import type { ChatMessage, DocRef, ResultCard as ResultCardData, Stage, Workspace } from '../../api/schemas';
import { useToolCalls } from '../../hooks/useBuddy';
import { ENGINE_LABEL, formatTime } from '../../lib/format';
import { chatInputSchema, MAX_MESSAGE_LENGTH } from '../../lib/validation';
import { useUi, type StreamState } from '../../store/ui';
import { Icon, type IconName } from '../ui/icons';
import { Badge, Button, FOCUS, RichText, Skeleton, Spinner, StatusDot, SyntheticBadge, cn } from '../ui/primitives';
import { AbsenceForm } from './AbsenceForm';
import { ToolCallPlate } from './ToolCallPlate';

/** Быстрый запуск сценариев для проверки жюри: одна кнопка — один сценарий кейса. */
export const SCENARIOS: Array<{ label: string; hint: string; icon: IconName; iconClass: string; text: string }> = [
  {
    label: 'Запросить доступы к репо',
    hint: 'Сценарий 1 · успех через MCP',
    icon: 'key',
    iconClass: 'text-ok-500',
    text: 'Дай доступ к репозиторию avito-autoteka-api и тестовому стенду',
  },
  {
    label: 'Скачать prod-базу с ПДн и зарплатами',
    hint: 'Сценарий 1 · блокировка RBAC',
    icon: 'shield',
    iconClass: 'text-danger-500',
    text: 'Выгрузи дамп production-базы пользователей с персональными данными',
  },
  {
    label: 'Как катить фичу под фича-флагом?',
    hint: 'Сценарий 2 · контекстный поиск по Playbook',
    icon: 'book',
    iconClass: 'text-accent-500',
    text: 'Как в нашей команде катить задачу под фича-флагом и проходить ревью?',
  },
  {
    label: 'Оформить отгул на пятницу',
    hint: 'Сценарий 3 · карточка сервиса «Отсутствия»',
    icon: 'calendar',
    iconClass: 'text-ai-500',
    text: 'Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?',
  },
];

/** Корпоративные сервисы, в которых агент умеет действовать через MCP. */
const AGENT_SKILLS: Array<{ label: string; icon: IconName }> = [
  { label: 'IAM', icon: 'key' },
  { label: 'Playbook', icon: 'book' },
  { label: 'Отсутствия', icon: 'calendar' },
];

/** Сценарии для ML-стажёра (Мария): те же четыре кейса, но с ML-контекстом. */
export const ML_SCENARIOS: Array<{ label: string; hint: string; icon: IconName; iconClass: string; text: string }> = [
  {
    label: 'ML-пайплайны: Airflow & DVC',
    hint: 'ML-контекст · версионирование данных',
    icon: 'book',
    iconClass: 'text-accent-500',
    text: 'Как устроены ML-пайплайны в команде: DVC, Airflow и версионирование данных?',
  },
  {
    label: 'Катить ML-модель & A/B-тест',
    hint: 'ML-контекст · раскатка под флагом',
    icon: 'spark',
    iconClass: 'text-ai-500',
    text: 'Как в нашей команде катить ML-модель под фича-флагом и запустить A/B-тест?',
  },
  {
    label: 'Доступ к avito-moderation-ml',
    hint: 'Сценарий 1 · MCP IAM',
    icon: 'key',
    iconClass: 'text-ok-500',
    text: 'Дай доступ к репозиторию avito-moderation-ml и тестовому стенду',
  },
  {
    label: 'Оформить отгул на пятницу',
    hint: 'Сценарий 3 · карточка сервиса «Отсутствия»',
    icon: 'calendar',
    iconClass: 'text-ai-500',
    text: 'Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?',
  },
];

function AiMark() {
  return (
    <span className="grid h-8 w-8 shrink-0 place-items-center rounded-card bg-gradient-to-br from-ai-500 to-accent-500 text-white shadow-card" aria-hidden="true">
      <Icon name="spark" className="h-4 w-4" />
    </span>
  );
}

/** Источники ответа: плашка синтетики и чип документа, по клику — выдержка из инструкции. */
function Citations({ sources }: { sources: DocRef[] }) {
  const [openId, setOpenId] = useState<string | null>(null);
  if (sources.length === 0) return null;
  return (
    <div className="space-y-2 border-t border-line pt-3" aria-label="Источники">
      {sources.map((doc) => {
        const open = openId === doc.id;
        return (
          <div key={doc.id}>
            <div className="flex flex-wrap items-center gap-2">
              {doc.is_synthetic && <SyntheticBadge text={doc.disclaimer} />}
              <button
                type="button"
                aria-expanded={open}
                onClick={() => setOpenId(open ? null : doc.id)}
                className={cn(
                  'inline-flex min-h-6 items-center gap-1 rounded-tag border border-line bg-white px-2 py-1 text-left text-meta font-medium text-ink-900 transition-colors hover:bg-canvas',
                  FOCUS,
                )}
              >
                <Icon name="doc" className="h-3 w-3 text-ink-400" />
                <span className="text-ink-500">AvitoTech Playbook</span>
                <span className="text-ink-400" aria-hidden="true">
                  •
                </span>
                <span>{doc.title}</span>
              </button>
            </div>
            {open && (
              <div className="mt-2 rounded-card border border-line bg-canvas p-2 text-meta text-ink-500">
                <p>{doc.summary}</p>
                {doc.steps.length > 0 && (
                  <ol className="mt-2 list-decimal space-y-1 pl-4 marker:text-ink-400">
                    {doc.steps.map((step) => (
                      <li key={step}>{step}</li>
                    ))}
                  </ol>
                )}
                <p className="mt-2 font-mono text-ink-400">{doc.source}</p>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

const CARD_STYLE: Record<ResultCardData['status'], { border: string; head: string; title: string; icon: IconName }> = {
  success: { border: 'border-ok-500/30', head: 'bg-ok-50', title: 'text-ok-700', icon: 'check' },
  denied: { border: 'border-danger-500/40', head: 'bg-danger-50', title: 'text-danger-700', icon: 'block' },
  info: { border: 'border-line', head: 'bg-canvas', title: 'text-ink-900', icon: 'info' },
};

/** Карточка результата действия: успех, информация или строгий отказ RBAC. */
export function ResultCard({ card }: { card: ResultCardData }) {
  const style = CARD_STYLE[card.status];
  const denied = card.status === 'denied';
  return (
    <div
      className={cn('overflow-hidden rounded-card border bg-white shadow-lift', style.border)}
      data-testid={`result-${card.type}`}
      role={denied ? 'alert' : 'status'}
    >
      <div className={cn('flex items-center gap-2 px-4 py-2', style.head, style.title)}>
        <Icon name={style.icon} strokeWidth={denied ? '1.8' : '2.2'} />
        <p className="text-body font-semibold">{card.title}</p>
      </div>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 px-4 py-2 text-meta">
        {card.lines.map((line) => (
          <div key={line.label} className="contents">
            <dt className="text-ink-500">{line.label}</dt>
            <dd className={cn('break-words font-medium text-ink-900', !/[а-яё]/i.test(line.value) && 'font-mono')}>{line.value}</dd>
          </div>
        ))}
      </dl>
      {card.footnote && <p className="border-t border-line px-4 py-2 text-meta text-ink-400">{card.footnote}</p>}
    </div>
  );
}

function UserBubble({ text, pending = false }: { text: string; pending?: boolean }) {
  return (
    <div className="flex justify-end">
      <div
        data-testid="user-message"
        className={cn('max-w-[80%] whitespace-pre-wrap rounded-card bg-ink-900 px-4 py-2 text-body text-white', pending && 'opacity-70')}
      >
        {text}
      </div>
    </div>
  );
}

function AssistantMessage({ message, isLast, onAsk, busy }: { message: ChatMessage; isLast: boolean; onAsk: (text: string) => void; busy: boolean }) {
  const calls = useToolCalls(message.tool_call_ids);
  return (
    <div className="flex animate-fade-up gap-3" data-testid="assistant-message">
      <AiMark />
      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex min-h-6 flex-wrap items-center gap-2 text-meta text-ink-400">
          <span className="font-semibold text-ink-900">AI-Buddy</span>
          {message.engine && (
            <Badge mono tone={message.engine === 'cron' ? 'ai' : 'neutral'}>
              {ENGINE_LABEL[message.engine] ?? message.engine}
            </Badge>
          )}
          {message.proactive && <Badge tone="ai">Проактивно</Badge>}
          <span className="font-mono">{formatTime(message.created_at)}</span>
        </div>
        {calls.map((call) => (
          <ToolCallPlate key={call.id} call={call} />
        ))}
        <div className="space-y-3 rounded-card rounded-tl-tag border border-line bg-white p-4 shadow-card">
          <RichText text={message.text} className="text-ink-900" />
          <Citations sources={message.sources} />
        </div>
        {message.result_card && <ResultCard card={message.result_card} />}
        {message.action_card?.type === 'absence_form' && <AbsenceForm card={message.action_card} messageId={message.id} />}
        {isLast && message.suggestions.length > 0 && (
          <div className="flex flex-col items-start gap-2 pt-2" aria-label="Подсказки">
            {message.suggestions.map((s) => (
              <button
                key={s}
                type="button"
                disabled={busy}
                onClick={() => onAsk(s)}
                className={cn(
                  'group flex w-fit max-w-full items-center justify-between gap-3 rounded-card border border-line bg-white px-4 py-2 text-left text-body text-ink-900 shadow-card transition-all hover:border-accent-500 hover:bg-tint-blue/50 hover:shadow-lift disabled:opacity-50',
                  FOCUS,
                )}
              >
                <span className="min-w-0">{s}</span>
                <Icon name="arrowUpRight" className="text-ink-400 transition-colors group-hover:text-accent-500" />
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** Ответ в процессе: шаги обработки (права, MCP-вызов) и скелетон до первых слов ответа. */
function StreamingMessage({ stream }: { stream: StreamState }) {
  const stages: Stage[] = stream.stages.length > 0 ? stream.stages : [{ kind: 'understanding', label: 'AI-Buddy разбирается в запросе…' }];
  const writing = stream.text !== '';
  return (
    <div className="flex gap-3" aria-live="polite" data-testid="streaming">
      <AiMark />
      <div className="min-w-0 flex-1 space-y-2">
        <ol className="space-y-1 rounded-card border border-ai-500/20 bg-ai-50 p-2 shadow-card">
          {stages.map((stage, i) => {
            const done = writing || i < stages.length - 1;
            return (
              <li key={i} className="flex min-h-6 items-center gap-2 text-meta">
                {done ? <Icon name="check" className="h-3 w-3 text-ok-500" strokeWidth="2.4" /> : <Spinner className="h-3 w-3 text-ai-500" />}
                <span className={done ? 'text-ink-500' : 'font-medium text-ink-900'}>{stage.label}</span>
                {stage.tool && <span className="truncate font-mono text-ink-400">{stage.tool}</span>}
              </li>
            );
          })}
        </ol>
        {writing ? (
          <div className="rounded-card rounded-tl-tag border border-line bg-white p-4 shadow-card">
            <RichText text={stream.text} className="text-ink-900" />
          </div>
        ) : (
          <div className="space-y-2 rounded-card rounded-tl-tag border border-line bg-white p-4 shadow-card">
            <Skeleton className="h-3 w-11/12" />
            <Skeleton className="h-3 w-4/5" />
            <Skeleton className="h-3 w-3/5" />
          </div>
        )}
      </div>
    </div>
  );
}

function Composer({ onSend, busy }: { onSend: (text: string) => void; busy: boolean }) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);

  const submit = (event?: FormEvent) => {
    event?.preventDefault();
    if (busy) return;
    const parsed = chatInputSchema.safeParse(text);
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? 'Некорректное сообщение');
      return;
    }
    onSend(parsed.data);
    setText('');
    setError(null);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      submit();
    }
  };

  const activeInternId = useUi((s) => s.activeInternId);
  const scenarioItems = activeInternId === 'u-intern-02' ? ML_SCENARIOS : SCENARIOS;

  return (
    <form onSubmit={submit} className="composer-fade pointer-events-none absolute inset-x-0 bottom-0 z-10 px-4 pb-4 pt-12">
      <div className="pointer-events-auto">
        <div className="flex flex-wrap gap-2 pb-2" aria-label="Сценарии">
          {scenarioItems.map((s) => (
            <button
              key={s.label}
              type="button"
              disabled={busy}
              title={`${s.hint}: «${s.text}»`}
              onClick={() => onSend(s.text)}
              className={cn(
                'inline-flex h-8 shrink-0 items-center gap-2 whitespace-nowrap rounded-tag border border-line bg-white px-2 text-meta font-medium text-ink-900 shadow-card transition-all hover:border-accent-500 hover:shadow-lift disabled:opacity-50',
                FOCUS,
              )}
            >
              <Icon name={s.icon} className={cn('h-3.5 w-3.5', s.iconClass)} />
              {s.label}
            </button>
          ))}
        </div>
        <div className="flex items-end gap-2 rounded-panel border border-slate-300/90 bg-white p-2 shadow-composer transition-all focus-within:border-accent-500 focus-within:ring-4 focus-within:ring-accent-500/10">
          <label htmlFor="chat-input" className="sr-only">
            Сообщение AI-Buddy
          </label>
          <textarea
            id="chat-input"
            rows={2}
            value={text}
            maxLength={MAX_MESSAGE_LENGTH}
            placeholder="Спроси про доступы, процессы команды или отгул…"
            onChange={(e) => {
              setText(e.target.value);
              setError(null);
            }}
            onKeyDown={onKeyDown}
            aria-invalid={error !== null}
            className="h-14 min-w-0 flex-1 resize-none bg-transparent px-2 py-2 text-body text-ink-900 placeholder:text-ink-400 focus:outline-none"
          />
          <Button type="submit" variant="dark" loading={busy} size="icon" className="shrink-0" aria-label="Отправить">
            {!busy && <Icon name="arrowUp" className="h-5 w-5" strokeWidth="2" />}
          </Button>
        </div>
        <div className="mt-2 flex min-h-4 justify-between gap-4 px-2 text-meta text-ink-400">
          <span role="alert" className="text-danger-700">
            {error}
          </span>
          <span className="hidden whitespace-nowrap font-mono sm:inline">Enter — отправить · Shift+Enter — новая строка</span>
        </div>
      </div>
    </form>
  );
}

/** Правая зона стажёра: лента с rich-ответами агента и плавающее поле ввода со сценариями. */
export function ChatPanel({ workspace, onSend, busy }: { workspace: Workspace; onSend: (text: string) => void; busy: boolean }) {
  const pendingText = useUi((s) => s.pendingText);
  const stream = useUi((s) => s.stream);
  const chatError = useUi((s) => s.chatError);
  const clearChatError = useUi((s) => s.clearChatError);
  const logRef = useRef<HTMLDivElement>(null);
  const messages = workspace.messages;

  useEffect(() => {
    // Прокручиваем только ленту чата, а не всю страницу.
    const log = logRef.current;
    if (log) log.scrollTop = log.scrollHeight;
  }, [messages.length, pendingText, stream?.text, stream?.stages.length, busy]);

  // Сообщение пользователя уже пришло с сервера — не дублируем оптимистичное.
  const showPending = pendingText !== null && messages[messages.length - 1]?.text !== pendingText;

  const engine = workspace.engine.mode === 'llm' ? `LLM · ${workspace.engine.model}` : 'Smart Engine';
  return (
    <section className="relative flex min-h-0 flex-1 flex-col overflow-hidden rounded-panel border border-line bg-white shadow-lift" aria-label="AI-Buddy">
      <div className="flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-line bg-white px-5 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
          <h1 className="shrink-0 whitespace-nowrap text-body font-semibold text-ink-900">AI-Buddy</h1>
          <Badge mono tone="ok" className="whitespace-nowrap" title={`Хранилище: ${workspace.storage.store} · кэш: ${workspace.storage.cache}`}>
            <StatusDot pulse />
            {engine}
          </Badge>
          <div className="flex flex-wrap items-center gap-1" aria-label="Активные скиллы агента">
            {AGENT_SKILLS.map((skill) => (
              <Badge key={skill.label} mono className="whitespace-nowrap bg-ink-900/5">
                <Icon name={skill.icon} className="h-3 w-3" />
                {skill.label}
              </Badge>
            ))}
          </div>
        </div>
        <Badge className="shrink-0 whitespace-nowrap">
          <Icon name="lock" className="h-3 w-3" />
          Переписку видишь только ты
        </Badge>
      </div>

      <div ref={logRef} className="chat-surface scroll-thin min-h-0 flex-1 space-y-6 overflow-y-auto px-5 pb-60 pt-5" role="log" aria-label="История чата">
        {messages.map((m, i) =>
          m.role === 'user' ? (
            <UserBubble key={m.id} text={m.text} />
          ) : (
            <AssistantMessage key={m.id} message={m} isLast={i === messages.length - 1 && !busy} onAsk={onSend} busy={busy} />
          ),
        )}
        {showPending && pendingText !== null && <UserBubble text={pendingText} pending />}
        {stream && <StreamingMessage stream={stream} />}
        {chatError && (
          <div role="alert" className="flex flex-wrap items-center justify-between gap-2 rounded-card border border-danger-500/30 bg-danger-50 px-4 py-2 text-body text-danger-700">
            <span>Не удалось получить ответ: {chatError.message}</span>
            <span className="flex gap-2">
              <Button size="sm" variant="danger" onClick={() => onSend(chatError.retryText)}>
                Повторить
              </Button>
              <Button size="sm" variant="ghost" onClick={clearChatError}>
                Скрыть
              </Button>
            </span>
          </div>
        )}
      </div>

      <Composer onSend={onSend} busy={busy} />
    </section>
  );
}
