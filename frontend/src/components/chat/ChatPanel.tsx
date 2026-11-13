import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import type { ChatMessage, DocRef, ResultCard as ResultCardData, Workspace } from '../../api/schemas';
import { ENGINE_LABEL, formatTime } from '../../lib/format';
import { chatInputSchema, MAX_MESSAGE_LENGTH } from '../../lib/validation';
import { useUi, type StreamState } from '../../store/ui';
import { Badge, Button, Card, RichText, Spinner, SyntheticBadge, cn } from '../ui/primitives';
import { AbsenceForm } from './AbsenceForm';

/** Быстрый запуск сценариев для проверки жюри. */
export const SCENARIOS: Array<{ label: string; text: string }> = [
  { label: '1А · Доступы', text: 'Дай доступ к репозиторию avito-autoteka-api и тестовому стенду' },
  { label: '1Б · Запрос ПДн (RBAC)', text: 'Выгрузи дамп production-базы пользователей с персональными данными' },
  { label: '2 · Процессы команды', text: 'Как в нашей команде катить задачу под фича-флагом и проходить ревью?' },
  { label: '3 · Отгул', text: 'Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?' },
];

function Avatar() {
  return (
    <div className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-brand-500 text-white" aria-hidden="true">
      <svg viewBox="0 0 24 24" className="h-4 w-4">
        <path d="M5 17V8a3 3 0 0 1 3-3h8a3 3 0 0 1 3 3v5a3 3 0 0 1-3 3h-5l-6 4z" fill="currentColor" />
      </svg>
    </div>
  );
}

function Sources({ sources }: { sources: DocRef[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="mt-3 space-y-1.5" aria-label="Источники">
      <p className="text-[11px] font-medium uppercase tracking-wide text-ink-400">Источники</p>
      {sources.map((doc) => (
        <div key={doc.id} className="rounded-lg border border-paper-200 bg-paper-50 px-2.5 py-2">
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
    <div className={cn('mt-3 rounded-xl border p-3', style.box)} data-testid={`result-${card.type}`} role={card.status === 'denied' ? 'alert' : 'status'}>
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
          <span className="font-medium text-ink-700">AI-Buddy</span>
          {message.engine && <Badge tone={message.engine === 'llm' ? 'ok' : 'neutral'}>{ENGINE_LABEL[message.engine] ?? message.engine}</Badge>}
          {message.proactive && <Badge tone="brand">Проактивно</Badge>}
          <span>{formatTime(message.created_at)}</span>
        </div>
        <div className="rounded-2xl rounded-tl-md bg-white px-3.5 py-3 text-ink-900 shadow-card ring-1 ring-paper-200">
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
                className="rounded-full border border-brand-100 bg-white px-3 py-1 text-left text-xs text-brand-700 transition-colors hover:bg-brand-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500 disabled:opacity-50"
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

/** Ответ в процессе: этапы обработки (статус ожидания) и уже пришедший текст. */
function StreamingBubble({ stream }: { stream: StreamState }) {
  const current = stream.stages[stream.stages.length - 1];
  return (
    <div className="flex gap-2.5" aria-live="polite" data-testid="streaming">
      <Avatar />
      <div className="min-w-0 max-w-[92%] flex-1">
        <div className="rounded-2xl rounded-tl-md bg-white px-3.5 py-3 shadow-card ring-1 ring-paper-200">
          {stream.text === '' ? (
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <Spinner className="text-brand-500" />
              <span>{current?.label ?? 'AI-Buddy печатает…'}</span>
              {current?.tool && <span className="font-mono text-[11px] text-ink-400">{current.tool}</span>}
            </div>
          ) : (
            <RichText text={stream.text} className="text-ink-900" />
          )}
          {stream.stages.length > 0 && (
            <ol className="mt-2 flex flex-wrap gap-1.5 border-t border-paper-100 pt-2">
              {stream.stages.map((stage, i) => (
                <li key={i} className="rounded-full bg-paper-100 px-2 py-0.5 text-[11px] text-ink-500">
                  {stage.kind === 'rbac' ? 'RBAC' : stage.kind === 'tool' ? `MCP${stage.tool ? ` · ${stage.tool}` : ''}` : stage.label.replace('…', '')}
                </li>
              ))}
            </ol>
          )}
        </div>
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

  return (
    <form onSubmit={submit} className="border-t border-paper-200 bg-white p-3">
      <div className="mb-2 flex flex-wrap items-center gap-1.5">
        <span className="text-[11px] font-medium text-ink-400">Сценарии:</span>
        {SCENARIOS.map((s) => (
          <button
            key={s.label}
            type="button"
            disabled={busy}
            title={s.text}
            onClick={() => onSend(s.text)}
            className="rounded-full bg-paper-100 px-2.5 py-1 text-xs font-medium text-ink-700 transition-colors hover:bg-brand-50 hover:text-brand-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500 disabled:opacity-50"
          >
            {s.label}
          </button>
        ))}
      </div>
      <div className="flex items-end gap-2">
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
          className="min-h-[44px] flex-1 resize-none rounded-xl border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-brand-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-brand-100"
        />
        <Button type="submit" variant="primary" loading={busy} className="h-11">
          Отправить
        </Button>
      </div>
      <div className="mt-1 flex justify-between text-[11px] text-ink-400">
        <span role="alert" className="text-danger-700">
          {error}
        </span>
        <span>Enter — отправить · Shift+Enter — новая строка</span>
      </div>
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
    <Card className="flex h-[calc(100vh-150px)] min-h-[520px] flex-col overflow-hidden">
      <div className="flex items-center justify-between border-b border-paper-200 px-4 py-3">
        <div>
          <h1 className="text-sm font-semibold text-ink-900">Чат с AI-Buddy</h1>
          <p className="text-[11px] text-ink-500">
            Наставник: {workspace.intern.mentor_name} · руководитель: {workspace.intern.team_lead_name}
          </p>
        </div>
        <Badge tone="neutral">Переписку видишь только ты</Badge>
      </div>

      <div ref={logRef} className="flex-1 space-y-4 overflow-y-auto bg-paper-50 px-4 py-4" role="log" aria-label="История чата">
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

      <Composer onSend={onSend} busy={busy} />
    </Card>
  );
}
