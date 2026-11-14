import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import type { ChatMessage, DocRef, ResultCard as ResultCardData, Workspace } from '../../api/schemas';
import { okrsFor } from '../../lib/avito';
import { formatDateTime, formatTime } from '../../lib/format';
import { chatInputSchema, MAX_MESSAGE_LENGTH } from '../../lib/validation';
import { useUi, type StreamState } from '../../store/ui';
import { GoffyAvatar } from '../GoffyAvatar';
import { Badge, Button, Card, OnlineDot, RichText, SyntheticBadge, cn } from '../ui/primitives';
import { AbsenceForm } from './AbsenceForm';

/** Быстрые вопросы над полем ввода: по одному на каждый сценарий кейса. */
export const SCENARIOS: Array<{ icon: string; label: string; text: string }> = [
  { icon: '🔑', label: 'Запросить доступы к Stash', text: 'Дай доступ к репозиторию avito-autoteka-api и тестовому стенду' },
  { icon: '🛡', label: 'Проверить доступ к prod-БД', text: 'Выгрузи дамп production-базы пользователей с персональными данными' },
  { icon: '📘', label: 'Выкатка под фича-флагом', text: 'Как в нашей команде катить задачу под фича-флагом и проходить ревью?' },
  { icon: '📅', label: 'Оформить отгул', text: 'Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?' },
];

const DEFAULT_REPOSITORY = 'avito-autoteka-api';

const ACCESS_STATUS: Record<string, string> = { completed: 'доступ выдан', in_progress: 'заявка в работе', pending: 'ждёт заявки' };

/** Контекст рабочей недели: цель, доступы и ближайшая встреча 1:1 одной строкой над перепиской. */
function WeekContext({ workspace }: { workspace: Workspace }) {
  const { intern, checklist, planner } = workspace;
  const goal = okrsFor(intern.stack)[0];
  const access = checklist.find((item) => item.id === 'access_repo');
  const oneOnOne = planner
    .filter((item) => item.title.includes('1:1') && item.status === 'planned')
    .sort((a, b) => a.starts_at.localeCompare(b.starts_at))[0];
  const cells: Array<{ label: string; value: string; note: string }> = [
    { label: 'Цель спринта', value: goal ? goal.text : 'Цели пока не заданы', note: `Задача ${workspace.sprint_task.key}` },
    { label: 'Доступы', value: `Stash: ${access ? ACCESS_STATUS[access.status] : 'нет данных'}`, note: intern.repository },
    {
      label: 'Ближайший 1:1',
      value: oneOnOne ? formatDateTime(oneOnOne.starts_at) : 'Не запланирован',
      note: `Наставник: ${intern.mentor_name}`,
    },
  ];
  return (
    <section className="grid gap-2 sm:grid-cols-3" aria-label="Контекст рабочей недели" data-testid="week-context">
      {cells.map((cell) => (
        <div key={cell.label} className="glass-inner min-w-0 px-3 py-2">
          <p className="text-[11px] text-ink-500">{cell.label}</p>
          <p className="mt-0.5 text-xs font-semibold leading-snug text-ink-900">{cell.value}</p>
          <p className="mt-0.5 truncate text-[11px] text-ink-400">{cell.note}</p>
        </div>
      ))}
    </section>
  );
}

const Avatar = () => <GoffyAvatar size="sm" />;

function Sources({ sources }: { sources: DocRef[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="mt-3 space-y-1.5" aria-label="Источники">
      <p className="text-[11px] font-medium uppercase tracking-wide text-ink-400">Источники</p>
      {sources.map((doc) => (
        <div key={doc.id} className="rounded-xl border border-paper-200 bg-paper-50 px-2.5 py-2">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-xs font-medium text-ink-900">{doc.title}</p>
            {doc.is_synthetic && <SyntheticBadge text={doc.disclaimer} />}
          </div>
          <p className="mt-0.5 text-[11px] text-ink-500">{doc.source}</p>
        </div>
      ))}
    </div>
  );
}

const CARD_STYLE: Record<ResultCardData['status'], { box: string; icon: string; path: string }> = {
  success: { box: 'border-ok-500/30 bg-ok-50', icon: 'bg-ok-500', path: 'm4 8.5 3 3 5.5-7' },
  denied: { box: 'border-danger-500/30 bg-danger-50', icon: 'bg-danger-500', path: 'M5 5l6 6M11 5l-6 6' },
  info: { box: 'border-paper-200 bg-paper-50', icon: 'bg-ink-500', path: 'M8 4.5v.5M8 7.5v4' },
};

export function ResultCard({ card }: { card: ResultCardData }) {
  const style = CARD_STYLE[card.status];
  return (
    <div className={cn('mt-3 rounded-2xl border p-3', style.box)} data-testid={`result-${card.type}`} role={card.status === 'denied' ? 'alert' : 'status'}>
      <div className="flex items-center gap-2">
        <span className={cn('grid h-5 w-5 place-items-center rounded-full text-white', style.icon)} aria-hidden="true">
          <svg viewBox="0 0 16 16" className="h-3 w-3">
            <path d={style.path} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </span>
        <p className="text-sm font-semibold text-ink-900">{card.title}</p>
      </div>
      <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        {card.lines.map((line) => (
          <div key={line.label} className="contents">
            <dt className="text-ink-500">{line.label}</dt>
            <dd className="break-words font-medium text-ink-900">{line.value}</dd>
          </div>
        ))}
      </dl>
      {card.footnote && <p className="mt-2 text-[11px] text-ink-500">{card.footnote}</p>}
    </div>
  );
}

function MessageBubble({ message, isLast, onAsk, busy }: { message: ChatMessage; isLast: boolean; onAsk: (text: string) => void; busy: boolean }) {
  if (message.role === 'user') {
    return (
      <div className="flex animate-fade-up justify-end">
        <div className="max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-md bg-brand-500 px-3.5 py-2.5 text-sm leading-relaxed text-white">{message.text}</div>
      </div>
    );
  }
  return (
    <div className="flex animate-fade-up gap-2.5" data-testid="assistant-message">
      <Avatar />
      <div className="min-w-0 max-w-[92%] flex-1">
        <div className="mb-1 flex flex-wrap items-center gap-1.5 text-[11px] text-ink-400">
          <span className="font-medium text-ink-700">Гоффи</span>
          {message.proactive && <Badge tone="brand">Проактивно</Badge>}
          <span>{formatTime(message.created_at)}</span>
        </div>
        <div className="rounded-2xl rounded-tl-md bg-paper-0 px-3.5 py-3 text-ink-900 shadow-card ring-1 ring-paper-200">
          <RichText text={message.text} />
          <Sources sources={message.sources} />
          {message.result_card && <ResultCard card={message.result_card} />}
          {message.action_card?.type === 'absence_form' && <AbsenceForm card={message.action_card} messageId={message.id} />}
        </div>
        {isLast && message.suggestions.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1.5" aria-label="Подсказки">
            {message.suggestions.map((s) => (
              <button
                key={s}
                type="button"
                disabled={busy}
                onClick={() => onAsk(s)}
                className="rounded-full border border-brand-100 bg-paper-0 px-3 py-1 text-left text-xs text-brand-700 transition-colors hover:bg-brand-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500 disabled:opacity-50"
              >
                {s}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** Ответ в процессе: дружелюбный статус ожидания и уже пришедший текст. Служебные детали — в «Инспекторе жюри». */
function StreamingBubble({ stream }: { stream: StreamState }) {
  const current = stream.stages[stream.stages.length - 1];
  return (
    <div className="flex animate-fade-up gap-2.5" aria-live="polite" data-testid="streaming">
      <Avatar />
      <div className="min-w-0 max-w-[92%] flex-1">
        <div className="rounded-2xl rounded-tl-md bg-paper-0 px-3.5 py-3 shadow-card ring-1 ring-paper-200">
          {stream.text === '' ? (
            <div className="flex items-center gap-2.5 text-sm text-ink-500">
              <span className="flex gap-1" aria-hidden="true">
                {[0, 1, 2].map((i) => (
                  <span key={i} className="h-1.5 w-1.5 animate-bounce rounded-full bg-brand-500" style={{ animationDelay: `${i * 120}ms` }} />
                ))}
              </span>
              <span>{current?.label ?? 'Гоффи печатает…'}</span>
            </div>
          ) : (
            <RichText text={stream.text} className="text-ink-900" />
          )}
        </div>
      </div>
    </div>
  );
}

function Composer({ onSend, busy, repository }: { onSend: (text: string) => void; busy: boolean; repository: string }) {
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

  return (
    <form onSubmit={submit} className="m-3 mt-0 shrink-0 rounded-2xl border border-paper-200 bg-paper-0 p-3 shadow-card focus-within:border-brand-500/50">
      <div className="mb-2 flex flex-wrap items-center gap-1.5" aria-label="Быстрые вопросы">
        {SCENARIOS.map((s) => (
          <button
            key={s.label}
            type="button"
            disabled={busy}
            title={s.text}
            onClick={() => onSend(s.text.replace(DEFAULT_REPOSITORY, repository))}
            className="rounded-full bg-paper-100 px-2.5 py-1 text-xs font-medium text-ink-700 transition-colors hover:bg-brand-50 hover:text-brand-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500 disabled:opacity-50"
          >
            <span aria-hidden="true">{s.icon} </span>
            {s.label}
          </button>
        ))}
      </div>
      <div className="flex items-end gap-2">
        <label htmlFor="chat-input" className="sr-only">
          Сообщение Гоффи
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
          className="min-h-[44px] flex-1 resize-none rounded-xl border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-brand-500 focus:bg-paper-0 focus:outline-none focus:ring-2 focus:ring-brand-100"
        />
        <Button type="submit" variant="primary" loading={busy} className="h-11">
          Отправить
        </Button>
      </div>
      <p role="alert" className="mt-1 min-h-4 text-[11px] text-danger-700">
        {error}
      </p>
    </form>
  );
}

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
  }, [messages.length, pendingText, stream?.text, stream?.stages.length]);

  // Сообщение пользователя уже пришло с сервера — не дублируем оптимистичное.
  const showPending = pendingText !== null && messages[messages.length - 1]?.text !== pendingText;

  return (
    <Card className="flex h-[70vh] min-h-[520px] flex-col overflow-hidden lg:h-full lg:min-h-0">
      <div className="flex shrink-0 items-center justify-between gap-3 border-b border-paper-200 px-4 py-3">
        <div className="flex items-center gap-3">
          <GoffyAvatar size="md" pulse />
          <div>
            <h1 className="flex items-center gap-2 text-sm font-semibold text-ink-900">
              Гоффи <OnlineDot /> <span className="text-[11px] font-normal text-ok-700">на связи</span>
            </h1>
            <p className="text-[11px] text-ink-500">
              Наставник: {workspace.intern.mentor_name} · руководитель: {workspace.intern.team_lead_name}
            </p>
          </div>
        </div>
        <Badge tone="brand" className="shrink-0 whitespace-nowrap">
          {workspace.intern.team} • {workspace.intern.stack}
        </Badge>
      </div>

      <div ref={logRef} className="min-h-0 flex-1 space-y-4 overflow-y-auto bg-paper-50/40 px-4 py-4" role="log" aria-label="История чата">
        <WeekContext workspace={workspace} />
        {messages.map((m, i) => (
          <MessageBubble key={m.id} message={m} isLast={i === messages.length - 1 && !busy} onAsk={onSend} busy={busy} />
        ))}
        {showPending && (
          <div className="flex justify-end">
            <div className="max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-md bg-brand-500/80 px-3.5 py-2.5 text-sm leading-relaxed text-white">{pendingText}</div>
          </div>
        )}
        {stream && <StreamingBubble stream={stream} />}
        {chatError && (
          <div role="alert" className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-danger-500/30 bg-danger-50 px-3 py-2.5 text-sm text-danger-700">
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

      <Composer onSend={onSend} busy={busy} repository={workspace.intern.repository} />
    </Card>
  );
}
