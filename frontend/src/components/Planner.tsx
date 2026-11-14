import { useEffect, useMemo, useState, type FormEvent } from 'react';
import type { PlannerItem, Reminder, Workspace } from '../api/schemas';
import { errorText, useExecuteAction } from '../hooks/useBuddy';
import { formatTime, todayIso } from '../lib/format';
import { useUi } from '../store/ui';
import { Badge, Button, Card, CardTitle, EmptyState, cn } from './ui/primitives';

const KIND_META: Record<PlannerItem['kind'], { label: string; dot: string; tone: 'brand' | 'accent' | 'ok' | 'warn' | 'neutral' }> = {
  meeting: { label: 'Созвон', dot: 'bg-brand-500', tone: 'brand' },
  review: { label: 'Код-ревью', dot: 'bg-accent-500', tone: 'accent' },
  sync: { label: 'Синк', dot: 'bg-ok-500', tone: 'ok' },
  deadline: { label: 'Дедлайн', dot: 'bg-warn-500', tone: 'warn' },
  task: { label: 'Задача', dot: 'bg-ink-400', tone: 'neutral' },
};

const WEEKDAYS = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'];
const MONTHS = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];

const dayKey = (d: Date) => todayIso(d);

/** Две недели, начиная с понедельника текущей. */
export function calendarDays(today: Date): Date[] {
  const start = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
  return Array.from({ length: 14 }, (_, i) => new Date(start.getFullYear(), start.getMonth(), start.getDate() + i));
}

function whenLabel(item: PlannerItem): string {
  const d = new Date(item.starts_at);
  const date = `${d.getDate()} ${MONTHS[d.getMonth()]}`;
  if (item.all_day) return `${date}, весь день`;
  const end = item.duration_min > 0 ? `–${formatTime(new Date(d.getTime() + item.duration_min * 60_000).toISOString())}` : '';
  return `${date}, ${formatTime(item.starts_at)}${end}`;
}

// ---------- Лента напоминаний ----------

export function Reminders({ reminders, onOpen }: { reminders: Reminder[]; onOpen: (itemId: string) => void }) {
  return (
    <Card className="overflow-hidden p-4">
      <CardTitle aside={<Badge tone={reminders.some((r) => r.urgent) ? 'danger' : 'neutral'}>{reminders.length}</Badge>}>Напоминания</CardTitle>
      {reminders.length === 0 ? (
        <p className="mt-3 text-xs text-ink-500">Пока тихо: Гоффи напомнит о встречах и дедлайнах заранее.</p>
      ) : (
        <ul className="mt-3 space-y-1.5" aria-label="Лента напоминаний">
          {reminders.map((r) => {
            const body = (
              <span className={cn('block rounded-2xl px-2.5 py-2 text-left text-xs leading-snug', r.urgent ? 'bg-danger-50 text-ink-900' : 'bg-paper-100 text-ink-700')}>
                {r.text}
              </span>
            );
            return (
              <li key={r.id} data-testid="reminder" className="animate-slide-in">
                {r.item_id ? (
                  <button
                    type="button"
                    onClick={() => onOpen(r.item_id!)}
                    className="block w-full rounded-xl transition-transform hover:-translate-y-0.5 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500"
                  >
                    {body}
                  </button>
                ) : (
                  body
                )}
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

/** Показывает всплывающее уведомление о новом срочном напоминании (по одному, без повторов). */
export function useReminderToasts(reminders: Reminder[] | undefined): void {
  const seen = useUi((s) => s.seenReminders);
  useEffect(() => {
    if (!reminders) return;
    const fresh = reminders.filter((r) => !seen.includes(r.id));
    if (fresh.length === 0) return;
    const ui = useUi.getState();
    const top = fresh.find((r) => r.urgent) ?? fresh[0]!;
    ui.markRemindersSeen(fresh.map((r) => r.id));
    ui.showToast({ kind: top.kind === 'absence_approved' ? 'success' : 'info', text: top.text });
  }, [reminders, seen]);
}

// ---------- Карточка события ----------

function ItemDialog({ item, canComplete, onClose }: { item: PlannerItem; canComplete: boolean; onClose: () => void }) {
  const [report, setReport] = useState('');
  const [error, setError] = useState<string | null>(null);
  const execute = useExecuteAction();
  const meta = KIND_META[item.kind];
  const done = item.status === 'done';

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const complete = (event: FormEvent) => {
    event.preventDefault();
    const args: Record<string, unknown> = { intern_id: item.intern_id, item_id: item.id };
    if (report.trim() !== '') args.report = report.trim();
    execute.mutate(
      { tool: 'calendar.complete_item', args },
      {
        onSuccess: () => {
          useUi.getState().showToast({ kind: 'success', text: report.trim() ? 'Отчёт отправлен Гоффи' : `Готово: «${item.title}»` });
          onClose();
        },
        onError: (e) => setError(errorText(e)),
      },
    );
  };

  return (
    <div className="fixed inset-0 z-40 grid place-items-center bg-black/50 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={item.title}
        onClick={(e) => e.stopPropagation()}
        className="bento w-full max-w-md animate-fade-up bg-paper-0 p-5"
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge tone={meta.tone}>{meta.label}</Badge>
            {item.mandatory && <Badge tone="danger">Обязательно</Badge>}
            {done && <Badge tone="ok">Выполнено</Badge>}
          </div>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="Закрыть карточку события">
            ✕
          </Button>
        </div>
        <h3 className="mt-3 text-base font-semibold text-ink-900">{item.title}</h3>
        <p className="mt-1 text-sm text-ink-500">{whenLabel(item)}</p>
        {item.description && <p className="mt-3 text-sm leading-relaxed text-ink-700">{item.description}</p>}
        <p className="mt-2 text-xs text-ink-400">Создал(а): {item.created_by_name || 'система'} · синтетические данные</p>

        {item.link && (
          <a
            href={item.link}
            target="_blank"
            rel="noreferrer noopener"
            className="mt-4 inline-flex items-center gap-2 rounded-xl bg-brand-500 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
          >
            <svg viewBox="0 0 20 20" className="h-4 w-4" aria-hidden="true">
              <path d="M3 6.5A1.5 1.5 0 0 1 4.5 5h7A1.5 1.5 0 0 1 13 6.5v7a1.5 1.5 0 0 1-1.5 1.5h-7A1.5 1.5 0 0 1 3 13.5zM14 8.5l3-2v7l-3-2z" fill="currentColor" />
            </svg>
            Открыть видеовстречу
          </a>
        )}
        {item.link && <p className="mt-1 text-[11px] text-ink-400">Тестовая ссылка (имитация сервиса видеовстреч)</p>}

        {done && item.report && (
          <div className="mt-4 rounded-2xl bg-ok-50 px-3 py-2.5 text-sm text-ink-900">
            <p className="text-[11px] font-medium uppercase tracking-wide text-ok-700">Отчёт для Гоффи</p>
            <p className="mt-1">{item.report}</p>
          </div>
        )}

        {!done && canComplete && (
          <form onSubmit={complete} className="mt-4 border-t border-paper-200 pt-4">
            <label htmlFor="item-report" className="block text-xs font-medium text-ink-700">
              Отчёт для Гоффи <span className="font-normal text-ink-400">— необязательно, попадёт в бриф наставнику</span>
            </label>
            <textarea
              id="item-report"
              rows={3}
              maxLength={1000}
              value={report}
              onChange={(e) => setReport(e.target.value)}
              placeholder="Что сделано, что осталось, где нужна помощь"
              className="mt-1 w-full resize-none rounded-xl border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-100"
            />
            {error && (
              <p role="alert" className="mt-2 text-xs text-danger-700">
                {error}
              </p>
            )}
            <Button type="submit" variant="primary" className="mt-3" loading={execute.isPending}>
              {report.trim() ? 'Отправить отчёт Гоффи' : 'Отметить выполненным'}
            </Button>
          </form>
        )}
      </div>
    </div>
  );
}

// ---------- Добавление задачи ----------

function AddTaskForm({ workspace, selectedDay }: { workspace: Workspace; selectedDay: string }) {
  const isIntern = workspace.viewer.role === 'intern_backend';
  const [title, setTitle] = useState('');
  const [time, setTime] = useState('');
  const [error, setError] = useState<string | null>(null);
  const execute = useExecuteAction();
  const today = todayIso();

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = title.trim();
    if (trimmed.length < 3) return setError('Название — минимум 3 символа');
    if (selectedDay < today) return setError('Выбери сегодняшний или будущий день');
    const args: Record<string, unknown> = { intern_id: workspace.intern.id, title: trimmed, date: selectedDay };
    if (time !== '') args.time = time;
    execute.mutate(
      { tool: 'calendar.add_task', args },
      {
        onSuccess: () => {
          setTitle('');
          setTime('');
          setError(null);
          useUi.getState().showToast({ kind: 'success', text: isIntern ? `Задача «${trimmed}» добавлена` : `Обязательная задача «${trimmed}» назначена стажёру` });
        },
        onError: (e) => setError(errorText(e)),
      },
    );
  };

  return (
    <form onSubmit={submit} className="mt-3 border-t border-paper-200 pt-3" aria-label={isIntern ? 'Новая личная задача' : 'Назначить обязательную задачу'}>
      <p className="text-xs font-medium text-ink-700">{isIntern ? 'Своя задача на выбранный день' : 'Назначить стажёру обязательную задачу'}</p>
      <div className="mt-1.5 flex gap-1.5">
        <label htmlFor="task-title" className="sr-only">
          Название задачи
        </label>
        <input
          id="task-title"
          value={title}
          maxLength={120}
          onChange={(e) => {
            setTitle(e.target.value);
            setError(null);
          }}
          placeholder={isIntern ? 'Настроить Go-окружение' : 'Пройти курс ИБ'}
          className="min-w-0 flex-1 rounded-xl border border-paper-200 bg-paper-50 px-2.5 py-1.5 text-sm text-ink-900 placeholder:text-ink-400 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-100"
        />
        <label htmlFor="task-time" className="sr-only">
          Время
        </label>
        <input
          id="task-time"
          type="time"
          value={time}
          onChange={(e) => setTime(e.target.value)}
          className="w-[92px] rounded-xl border border-paper-200 bg-paper-50 px-2 py-1.5 text-sm text-ink-900 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-100"
        />
      </div>
      {error && (
        <p role="alert" className="mt-1.5 text-[11px] text-danger-700">
          {error}
        </p>
      )}
      <Button type="submit" size="sm" variant="primary" className="mt-2" loading={execute.isPending}>
        {isIntern ? 'Добавить задачу' : 'Назначить'}
      </Button>
    </form>
  );
}

// ---------- Календарь ----------

export function Planner({ workspace }: { workspace: Workspace }) {
  const today = todayIso();
  const [selected, setSelected] = useState(today);
  const [openId, setOpenId] = useState<string | null>(null);
  const days = useMemo(() => calendarDays(new Date()), [today]); // eslint-disable-line react-hooks/exhaustive-deps
  const byDay = useMemo(() => {
    const map = new Map<string, PlannerItem[]>();
    for (const item of workspace.planner) {
      const key = dayKey(new Date(item.starts_at));
      map.set(key, [...(map.get(key) ?? []), item]);
    }
    return map;
  }, [workspace.planner]);

  const dayItems = byDay.get(selected) ?? [];
  const opened = workspace.planner.find((p) => p.id === openId) ?? null;
  const selectedDate = days.find((d) => dayKey(d) === selected);
  const canAdd = workspace.permissions.planner_add === true;

  return (
    <>
      {workspace.viewer.role === 'intern_backend' && <Reminders reminders={workspace.reminders} onOpen={setOpenId} />}

      <Card className="overflow-hidden p-4">
        <CardTitle aside={<span className="text-xs text-ink-500">2 недели</span>}>Календарь и задачи</CardTitle>

        <div className="mt-3 grid grid-cols-7 gap-1 text-center" role="grid" aria-label="Календарь на две недели">
          {WEEKDAYS.map((w) => (
            <span key={w} className="text-[10px] font-medium uppercase tracking-wide text-ink-400">
              {w}
            </span>
          ))}
          {days.map((d) => {
            const key = dayKey(d);
            const items = byDay.get(key) ?? [];
            const isToday = key === today;
            const isSelected = key === selected;
            return (
              <button
                key={key}
                type="button"
                role="gridcell"
                aria-selected={isSelected}
                aria-label={`${d.getDate()} ${MONTHS[d.getMonth()]}${items.length ? `, событий: ${items.length}` : ''}`}
                onClick={() => setSelected(key)}
                className={cn(
                  'relative flex h-11 flex-col items-center justify-center rounded-xl text-sm transition-all duration-200',
                  'focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500',
                  isSelected ? 'bg-brand-500 font-semibold text-white shadow-glow' : 'text-ink-700 hover:bg-paper-100',
                  key < today && !isSelected && 'text-ink-300',
                  isToday && !isSelected && 'ring-1 ring-brand-500/50',
                )}
              >
                {d.getDate()}
                <span className="mt-0.5 flex h-1.5 gap-0.5">
                  {items.slice(0, 3).map((it) => (
                    <span
                      key={it.id}
                      className={cn('h-1.5 w-1.5 rounded-full', isSelected ? 'bg-white' : KIND_META[it.kind].dot, it.status === 'done' && 'opacity-40')}
                    />
                  ))}
                </span>
              </button>
            );
          })}
        </div>

        <p className="mt-3 text-xs font-medium text-ink-700">
          {selectedDate ? `${selectedDate.getDate()} ${MONTHS[selectedDate.getMonth()]}` : selected}
          {selected === today && ' · сегодня'}
        </p>
        {dayItems.length === 0 ? (
          <div className="mt-2">
            <EmptyState title="На этот день ничего не запланировано" />
          </div>
        ) : (
          <ul className="mt-2 space-y-1.5" aria-label="События дня">
            {dayItems.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  data-testid="planner-item"
                  onClick={() => setOpenId(item.id)}
                  className="flex w-full items-center gap-2.5 rounded-2xl bg-paper-100 px-2.5 py-2 text-left transition-all duration-200 hover:-translate-y-0.5 hover:ring-1 hover:ring-brand-500/30 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500"
                >
                  <span className={cn('h-8 w-1 shrink-0 rounded-full', KIND_META[item.kind].dot)} aria-hidden="true" />
                  <span className="min-w-0 flex-1">
                    <span className={cn('block truncate text-sm font-medium', item.status === 'done' ? 'text-ink-400 line-through' : 'text-ink-900')}>{item.title}</span>
                    <span className="block text-[11px] text-ink-500">
                      {item.all_day ? 'весь день' : formatTime(item.starts_at)} · {KIND_META[item.kind].label}
                      {item.mandatory ? ' · обязательно' : ' · личное'}
                    </span>
                  </span>
                  {item.status === 'done' && (
                    <span className="grid h-5 w-5 shrink-0 animate-pop place-items-center rounded-full bg-ok-500 text-white" aria-label="выполнено">
                      <svg viewBox="0 0 16 16" className="h-3 w-3" aria-hidden="true">
                        <path d="m3.5 8.5 3 3 6-7" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                      </svg>
                    </span>
                  )}
                </button>
              </li>
            ))}
          </ul>
        )}

        {canAdd && <AddTaskForm workspace={workspace} selectedDay={selected} />}
      </Card>

      {opened && <ItemDialog item={opened} canComplete={workspace.permissions.planner_complete === true} onClose={() => setOpenId(null)} />}
    </>
  );
}
