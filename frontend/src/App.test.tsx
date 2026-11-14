import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import { setApiRole } from './api/auth';
import { SCENARIOS } from './components/chat/ChatPanel';
import { GOFFY_IMAGE_URL, GoffyAvatar, resetGoffyImageCache } from './components/GoffyAvatar';
import { calendarDays } from './components/Planner';
import { applyTheme, useUi } from './store/ui';
import { chatStream, fakeFetch, fixtures, json, type Call, type Route } from './test/helpers';

type ChatFixture = typeof fixtures.chatAccess;
type Json = Record<string, unknown>;

/** Имитация бэкенда с состоянием: ответы — реальные фикстуры API. */
function backend(overrides: Route = () => undefined) {
  let intern: Json = structuredClone(fixtures.workspaceInitial);
  const byText: Record<string, unknown> = {
    [SCENARIOS[0]!.text]: fixtures.chatAccess,
    [SCENARIOS[1]!.text]: fixtures.chatPii,
    [SCENARIOS[2]!.text]: fixtures.chatProcess,
    [SCENARIOS[3]!.text]: fixtures.chatAbsence,
  };
  const byTool: Record<string, unknown> = {
    'absences.create_day_off': fixtures.executeAbsence,
    'calendar.add_task': fixtures.executeAddTask,
    'calendar.complete_item': fixtures.executeReport,
    'absences.approve_day_off': fixtures.executeApprove,
  };
  const apply = (result: ChatFixture) => {
    const messages = [...(intern.messages as unknown[]), result.user_message, result.message].filter(Boolean);
    intern = { ...intern, messages, checklist: result.checklist, progress: result.progress, next_step: result.next_step };
  };
  const route: Route = (call: Call) => {
    const custom = overrides(call);
    if (custom) return custom;
    const { method, path, token } = call;
    if (method === 'GET' && path === '/api/v1/workspace') {
      if (token === 'demo-mentor-token') return json(fixtures.workspaceMentor);
      if (token === 'demo-lead-token') return json(fixtures.workspaceLead);
      return json(intern);
    }
    if (path === '/api/v1/chat/stream') {
      const result = (byText[(call.body as { message: string }).message] ?? fixtures.chatAccess) as ChatFixture;
      apply(result);
      return chatStream(result);
    }
    if (path === '/api/v1/mcp/execute') {
      const body = call.body as { tool: string; arguments: Json };
      const result = byTool[body.tool] as ChatFixture;
      if (token === 'demo-intern-token') {
        apply(result);
        const item = (result.tool_calls[0] as unknown as { result: Json }).result;
        const planner = intern.planner as Json[];
        if (body.tool === 'calendar.add_task') intern = { ...intern, planner: [...planner, item] };
        if (body.tool === 'calendar.complete_item') intern = { ...intern, planner: planner.map((p) => (p.id === item.id ? item : p)) };
      }
      return json(result);
    }
    if (path === '/api/v1/cron/trigger') return json(fixtures.cron);
    if (path === '/api/v1/reset') {
      intern = structuredClone(fixtures.workspaceInitial);
      return json(intern);
    }
    if (path === '/api/v1/telemetry') return json(fixtures.telemetry);
    if (path.startsWith('/api/v1/feature-flags/')) return json({ ...fixtures.flag, enabled: (call.body as { enabled: boolean }).enabled });
    if (path === '/api/v1/events') return json({ id: 'evt' }, 202);
    return undefined;
  };
  const fake = fakeFetch(route);
  vi.stubGlobal('fetch', fake.fn);
  return fake;
}

function renderApp() {
  const client = new QueryClient();
  return render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
}

const step = (id: string) => screen.getByTestId(`step-${id}`);
const chatLog = () => screen.getByRole('log', { name: 'История чата' });
const executeCalls = (calls: Call[]) => calls.filter((c) => c.path === '/api/v1/mcp/execute');

beforeEach(() => {
  // Фиксируем «сегодня» = день, когда сняты фикстуры: календарь и форма отгула зависят от даты.
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(2026, 10, 14, 10, 0, 0));
  useUi.setState({ role: 'intern', theme: 'light', inspectorOpen: false, inspectorTab: 'mcp', toast: null });
  useUi.getState().resetSession();
  applyTheme('light');
  setApiRole('intern');
  resetGoffyImageCache('missing');
});

afterEach(() => {
  vi.useRealTimers();
});

describe('Гоффи: рабочее пространство стажёра', () => {
  it('показывает маскота, чек-лист, контекст и дисклеймер синтетики', async () => {
    backend();
    renderApp();
    expect(screen.getByText(/Гоффи загружает рабочее пространство/)).toBeInTheDocument();
    expect(await screen.findByText('Чек-лист адаптации')).toBeInTheDocument();
    expect(screen.getByText('Команда Autoteka')).toBeInTheDocument();
    expect(screen.getByText('Week 1')).toBeInTheDocument();
    expect(screen.getByText(/Синтетические данные: сотрудники/)).toBeInTheDocument();
    expect(step('access_repo')).toHaveAttribute('data-status', 'pending');
    expect(within(step('access_repo')).getByText('Следующий шаг')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '2');

    // Гоффи — аватар в шапке, в чате и на приветственном экране.
    expect(screen.getByText(/Привет, Алексей! Я Гоффи/)).toBeInTheDocument();
    const hero = screen.getByTestId('goffy-hero');
    expect(within(hero).getByRole('img', { name: 'Гоффи' })).toHaveAttribute('data-size', 'lg');
    expect(screen.getAllByRole('img', { name: 'Гоффи' }).length).toBeGreaterThanOrEqual(4);
    expect(screen.queryByText(/AI-Buddy/)).not.toBeInTheDocument();
  });

  it('сценарий 1А: доступ выдан, шаг №1 выполнен, служебных деталей в чате нет', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '1А · Доступы' }));

    const card = await screen.findByTestId('result-access_granted');
    expect(within(card).getByText('Доступ выдан')).toBeInTheDocument();
    expect(within(card).getByText('avito-autoteka-api')).toBeInTheDocument();
    await waitFor(() => expect(step('access_repo')).toHaveAttribute('data-status', 'completed'));
    expect(within(step('team_processes')).getByText('Следующий шаг')).toBeInTheDocument();
    expect(calls.find((c) => c.path === '/api/v1/chat/stream')?.body).toEqual({ message: SCENARIOS[0]!.text });
    await waitFor(() => expect(screen.queryByTestId('streaming')).not.toBeInTheDocument());
    expect(screen.queryByTestId('goffy-hero')).not.toBeInTheDocument();

    // Телеметрия (MCP, RBAC, политики) стажёру не показывается.
    expect(chatLog()).not.toHaveTextContent(/POL-|ALLOWED|RBAC|MCP|iam_access|intern_backend|Smart Engine/);
  });

  it('сценарий 1Б: запрос заблокирован, ответ дружелюбный и без кодов', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '1Б · Запрос ПДн (RBAC)' }));

    const card = await screen.findByTestId('result-rbac_denied');
    expect(card).toHaveAttribute('role', 'alert');
    expect(within(card).getByText('Действие недоступно')).toBeInTheDocument();
    expect(within(card).getByText('Тестовый стенд команды с синтетическими данными')).toBeInTheDocument();
    expect(screen.getByText(/безопасная песочница/)).toBeInTheDocument();
    expect(chatLog()).not.toHaveTextContent(/POL-SEC|FORBIDDEN|403|db_admin/);
    expect(fixtures.chatPii.checklist_updates).toEqual([]); // отказ не двигает чек-лист
    // При этом вердикт остаётся в ответе API — его показывает «Инспектор жюри».
    expect(fixtures.chatPii.rbac).toMatchObject({ code: 'FORBIDDEN', http_status: 403, policy_id: 'POL-SEC-001' });
  });

  it('сценарий 2: источники помечены плашкой «Тестовая инструкция»', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '2 · Процессы команды' }));

    const sources = await screen.findByLabelText('Источники');
    expect(within(sources).getAllByText('Тестовая инструкция (Синтетические данные)')).toHaveLength(2);
    expect(within(sources).getByText('Как катить задачу под фича-флагом')).toBeInTheDocument();
    await waitFor(() => expect(step('team_processes')).toHaveAttribute('data-status', 'completed'));
  });

  it('сценарий 3: карточка отгула валидируется и отправляется', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '3 · Отгул' }));

    const form = await screen.findByRole('form', { name: 'Заявка на отгул' });
    expect(within(form).getByText('Ждёт подтверждения')).toBeInTheDocument();
    expect(within(form).getByLabelText('Дата')).toHaveValue('2026-11-20');
    expect(within(form).getByLabelText('С')).toHaveValue('14:00');
    expect(within(form)).not.toHaveProperty('textContent', expect.stringContaining('absences.create_day_off'));

    // Клиентская валидация (Zod) до обращения к серверу.
    const reason = within(form).getByLabelText('Причина');
    await user.clear(reason);
    await user.click(within(form).getByRole('button', { name: 'Оформить отгул' }));
    expect(within(form).getByRole('alert')).toHaveTextContent('минимум 3 символа');
    expect(executeCalls(calls)).toHaveLength(0);

    await user.type(reason, 'Зачёт в университете');
    await user.click(within(form).getByLabelText('Руководителя'));
    await user.click(within(form).getByRole('button', { name: 'Оформить отгул' }));

    const done = await screen.findByTestId('result-absence_created');
    expect(within(done).getByText('#ABS-2026-84')).toBeInTheDocument();
    expect(executeCalls(calls)[0]?.body).toMatchObject({
      tool: 'absences.create_day_off',
      message_id: fixtures.chatAbsence.message.id,
      arguments: { intern_id: 'u-intern-01', date: '2026-11-20', time_from: '14:00', time_to: '18:00', reason: 'Зачёт в университете', notify: ['mentor'] },
    });
    await waitFor(() => expect(within(form).getByText('Отправлено')).toBeInTheDocument());
    expect(within(form).getByLabelText('Дата')).toBeDisabled();
    await waitFor(() => expect(step('absence_rules')).toHaveAttribute('data-status', 'completed'));
  });

  it('показывает ошибку сервиса «Отсутствия» в карточке', async () => {
    backend(({ path }) => (path === '/api/v1/mcp/execute' ? json(fixtures.executeForbidden, 403) : undefined));
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '3 · Отгул' }));
    const form = await screen.findByRole('form', { name: 'Заявка на отгул' });
    await user.click(within(form).getByRole('button', { name: 'Оформить отгул' }));
    expect(await within(form).findByRole('alert')).toHaveTextContent(/default deny/);
    expect(within(form).getByRole('button', { name: 'Оформить отгул' })).toBeEnabled();
  });

  it('отправляет сообщение из поля ввода и по подсказке, проверяет пустой ввод', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    const input = await screen.findByLabelText('Сообщение Гоффи');

    await user.click(screen.getByRole('button', { name: 'Отправить' }));
    expect(screen.getByText('Напиши вопрос')).toBeInTheDocument();

    await user.type(input, 'Дай доступ{Shift>}{Enter}{/Shift}к репо{Enter}');
    await screen.findByTestId('result-access_granted');
    expect(calls.find((c) => c.path === '/api/v1/chat/stream')?.body).toEqual({ message: 'Дай доступ\nк репо' });
    expect(input).toHaveValue('');

    const suggestions = await screen.findByLabelText('Подсказки');
    await user.click(within(suggestions).getAllByRole('button')[0]!);
    await waitFor(() => expect(calls.filter((c) => c.path === '/api/v1/chat/stream')).toHaveLength(2));
  });

  it('кнопка «Спросить Гоффи» у следующего шага запускает сценарий', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: 'Спросить Гоффи' }));
    await screen.findByTestId('result-access_granted');
    expect(calls.find((c) => c.path === '/api/v1/chat/stream')?.body).toEqual({ message: SCENARIOS[0]!.text });
  });

  it('пока Гоффи отвечает, виден дружелюбный статус без названий инструментов', async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    backend(({ path }) => {
      if (path !== '/api/v1/chat/stream') return undefined;
      const encoder = new TextEncoder();
      const stream = new ReadableStream<Uint8Array>({
        async start(controller) {
          controller.enqueue(encoder.encode('event: stage\ndata: {"kind":"rbac","label":"Проверяю, можно ли это сделать…","tool":"iam_access.request_repo_access"}\n\n'));
          await gate;
          controller.enqueue(encoder.encode(`event: done\ndata: ${JSON.stringify(fixtures.chatAccess)}\n\n`));
          controller.close();
        },
      });
      return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } });
    });
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '1А · Доступы' }));
    const bubble = await screen.findByTestId('streaming');
    await waitFor(() => expect(bubble).toHaveTextContent('Проверяю, можно ли это сделать…'));
    expect(bubble).not.toHaveTextContent(/iam_access|MCP|RBAC/);
    release();
    await waitFor(() => expect(screen.queryByTestId('streaming')).not.toBeInTheDocument());
  });

  it('при сбое показывает ошибку, даёт повторить или скрыть', async () => {
    let fail = true;
    backend(({ path }) => {
      if (path === '/api/v1/chat/stream' && fail) return json({ error: { code: 'INTERNAL', message: 'Внутренняя ошибка сервера. Попробуйте ещё раз.' } }, 500);
      return undefined;
    });
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: '1А · Доступы' }));
    expect(await screen.findByText(/Не удалось получить ответ: Внутренняя ошибка сервера/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Скрыть' }));
    expect(screen.queryByText(/Не удалось получить ответ/)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '1А · Доступы' }));
    await screen.findByText(/Не удалось получить ответ/);
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Повторить' }));
    await screen.findByTestId('result-access_granted');
    expect(screen.queryByText(/Не удалось получить ответ/)).not.toBeInTheDocument();
  });

  it('запускает ИИ-крон и сбрасывает демо', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: 'Запустить ИИ-крон' }));
    const statuses = await screen.findAllByText(/стажёру отправлено проактивное сообщение/);
    expect(statuses.length).toBeGreaterThanOrEqual(1);
    await user.click(screen.getByRole('button', { name: 'Скрыть уведомление' }));

    await user.click(screen.getByRole('button', { name: 'Сбросить демо' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.path === '/api/v1/reset')).toBe(true));
  });

  it('ошибка загрузки: сообщение и повтор', async () => {
    let fail = true;
    backend(({ path }) => (path === '/api/v1/workspace' && fail ? json({ error: { code: 'INTERNAL', message: 'База недоступна' } }, 500) : undefined));
    const user = userEvent.setup();
    renderApp();
    expect(await screen.findByText('Не удалось загрузить рабочее пространство', {}, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.getByText('База недоступна')).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Повторить' }));
    expect(await screen.findByText('Чек-лист адаптации')).toBeInTheDocument();
  });
});

describe('статистика стажёра', () => {
  it('вкладка показывает прогресс, самостоятельность, время, доступы и материалы', async () => {
    backend(({ method, path, token }) =>
      method === 'GET' && path === '/api/v1/workspace' && token === 'demo-intern-token' ? json(fixtures.workspaceIntern) : undefined,
    );
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('tab', { name: 'Статистика' }));

    const m = fixtures.workspaceIntern.metrics;
    expect(screen.getByText('Прогресс адаптации')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', String(fixtures.workspaceIntern.progress.completed));
    expect(screen.getByTestId('deflection-rate')).toHaveTextContent(`${m.deflection_rate}%`);
    expect(screen.getByText(/Гоффи закрыл без отвлечения наставника/)).toBeInTheDocument();
    expect(screen.getByTestId('minutes-saved')).toHaveTextContent(/ч|мин/);
    expect(screen.getByText('доступов получено').previousSibling).toHaveTextContent(String(m.access_grants));
    expect(screen.getByText('материалов изучено').previousSibling).toHaveTextContent(`${m.docs_read}/${m.docs_total}`);
    expect(screen.getByText('событий календаря закрыто').previousSibling).toHaveTextContent(`${m.tasks_done}/${m.tasks_total}`);
    expect(screen.queryByText('Чек-лист адаптации')).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'Адаптация' }));
    expect(screen.getByText('Чек-лист адаптации')).toBeInTheDocument();
  });

  it('без вопросов показатель самостоятельности пуст', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('tab', { name: 'Статистика' }));
    expect(screen.getByTestId('deflection-rate')).toHaveTextContent('—');
    expect(screen.getByTestId('minutes-saved')).toHaveTextContent('0 мин');
    expect(screen.getByText(/Пока нет вопросов/)).toBeInTheDocument();
  });
});

describe('календарь, задачи и напоминания', () => {
  it('calendarDays возвращает две недели с понедельника', () => {
    const days = calendarDays(new Date(2026, 10, 14)); // суббота
    expect(days).toHaveLength(14);
    expect([days[0]!.getDate(), days[0]!.getDay(), days[13]!.getDate()]).toEqual([9, 1, 22]);
    expect(calendarDays(new Date(2026, 10, 9))[0]!.getDate()).toBe(9); // понедельник
    expect(calendarDays(new Date(2026, 10, 15))[0]!.getDate()).toBe(9); // воскресенье
  });

  it('показывает ленту напоминаний и всплывающее уведомление о ближайшем созвоне', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    const feed = await screen.findByLabelText('Лента напоминаний');
    const items = within(feed).getAllByTestId('reminder');
    expect(items).toHaveLength(fixtures.workspaceInitial.reminders.length);
    expect(items[0]).toHaveTextContent('🚨 Через 3 часа — Созвон 1:1 с наставником');
    expect(feed).toHaveTextContent('📅 Срок сдачи первого PR — в пятницу');

    // Toast показывается один раз для самого срочного напоминания.
    const toast = await screen.findByTestId('toast');
    expect(toast).toHaveTextContent('🚨 Через 3 часа — Созвон 1:1 с наставником');
    await user.click(within(toast).getByRole('button', { name: 'Скрыть уведомление' }));
    expect(screen.queryByTestId('toast')).not.toBeInTheDocument();
    expect(useUi.getState().seenReminders).toHaveLength(items.length);

    // Клик по напоминанию открывает карточку события.
    await user.click(within(items[0]!).getByRole('button'));
    expect(await screen.findByRole('dialog', { name: 'Созвон 1:1 с наставником' })).toBeInTheDocument();
  });

  it('сообщает о согласованном отгуле', async () => {
    backend(({ method, path, token }) =>
      method === 'GET' && path === '/api/v1/workspace' && token === 'demo-intern-token' ? json(fixtures.workspaceIntern) : undefined,
    );
    renderApp();
    const feed = await screen.findByLabelText('Лента напоминаний');
    expect(feed).toHaveTextContent('🔔 Руководитель согласовал твой отгул #ABS-2026-84');
  });

  it('карточка события: детали, ссылка на встречу, отчёт Гоффи', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    const grid = await screen.findByRole('grid', { name: 'Календарь на две недели' });
    expect(within(grid).getAllByRole('gridcell')).toHaveLength(14);
    expect(within(grid).getByRole('gridcell', { name: /^14 ноября/ })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('Созвон 1:1 с наставником', { selector: '[data-testid=planner-item] *' })).toBeInTheDocument();

    await user.click(within(grid).getByRole('gridcell', { name: '15 ноября, событий: 1' }));
    await user.click(within(screen.getByLabelText('События дня')).getByRole('button', { name: /Командный синк Autoteka/ }));

    const dialog = await screen.findByRole('dialog', { name: 'Командный синк Autoteka' });
    expect(within(dialog).getByText('Синк')).toBeInTheDocument();
    expect(within(dialog).getByText('Обязательно')).toBeInTheDocument();
    const link = within(dialog).getByRole('link', { name: 'Открыть видеовстречу' });
    expect(link).toHaveAttribute('href', 'https://meet.avito.example/autoteka-daily');
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(within(dialog).getByRole('button', { name: 'Отметить выполненным' })).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText(/Отчёт для Гоффи/), 'Синк прошёл, блокеров нет');
    await user.click(within(dialog).getByRole('button', { name: 'Отправить отчёт Гоффи' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(executeCalls(calls)[0]?.body).toMatchObject({
      tool: 'calendar.complete_item',
      arguments: { intern_id: 'u-intern-01', item_id: 'plan-sync', report: 'Синк прошёл, блокеров нет' },
    });
    // Гоффи подтверждает получение отчёта в чате, событие отмечено выполненным.
    expect(await screen.findByTestId('result-report_sent')).toHaveTextContent('Синк прошёл, блокеров нет');
    await waitFor(() => expect(screen.getByLabelText('выполнено')).toBeInTheDocument());

    // Выполненное событие: отчёт виден, формы нет; Escape закрывает карточку.
    await user.click(within(screen.getByLabelText('События дня')).getByRole('button', { name: /Командный синк Autoteka/ }));
    const reopened = await screen.findByRole('dialog');
    expect(within(reopened).getByText('Выполнено')).toBeInTheDocument();
    expect(within(reopened).queryByRole('button', { name: /Отметить|Отправить отчёт/ })).not.toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('карточка события закрывается кнопкой и кликом по фону, показывает ошибку сервера', async () => {
    backend(({ path }) => (path === '/api/v1/mcp/execute' ? json({ error: { code: 'INVALID_ARGUMENTS', message: 'событие не найдено' } }, 422) : undefined));
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByTestId('planner-item'));
    let dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Отметить выполненным' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('событие не найдено');
    await user.click(within(dialog).getByRole('button', { name: 'Закрыть карточку события' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    await user.click(screen.getByTestId('planner-item'));
    dialog = await screen.findByRole('dialog');
    await user.click(dialog.parentElement!);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('стажёр добавляет личную задачу: валидация и отправка', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    const form = await screen.findByRole('form', { name: 'Новая личная задача' });

    await user.type(within(form).getByLabelText('Название задачи'), 'ок');
    await user.click(within(form).getByRole('button', { name: 'Добавить задачу' }));
    expect(within(form).getByRole('alert')).toHaveTextContent('минимум 3 символа');

    // Прошедший день выбрать можно, но задачу на него добавить нельзя.
    await user.click(screen.getByRole('gridcell', { name: /^10 ноября/ }));
    expect(screen.getByText('На этот день ничего не запланировано')).toBeInTheDocument();
    await user.clear(within(form).getByLabelText('Название задачи'));
    await user.type(within(form).getByLabelText('Название задачи'), 'Настроить Go-окружение');
    await user.click(within(form).getByRole('button', { name: 'Добавить задачу' }));
    expect(within(form).getByRole('alert')).toHaveTextContent('сегодняшний или будущий');
    expect(executeCalls(calls)).toHaveLength(0);

    await user.click(screen.getByRole('gridcell', { name: /^14 ноября/ }));
    await user.type(within(form).getByLabelText('Время'), '12:30');
    await user.click(within(form).getByRole('button', { name: 'Добавить задачу' }));

    await waitFor(() =>
      expect(executeCalls(calls)[0]?.body).toMatchObject({
        tool: 'calendar.add_task',
        arguments: { intern_id: 'u-intern-01', title: 'Настроить Go-окружение', date: '2026-11-14', time: '12:30' },
      }),
    );
    expect(await screen.findByText('Задача «Настроить Go-окружение» добавлена')).toBeInTheDocument();
    expect(within(form).getByLabelText('Название задачи')).toHaveValue('');
    expect(await screen.findByRole('button', { name: /Настроить Go-окружение.*личное/ })).toBeInTheDocument();
    // Личная задача не засоряет чат.
    expect(chatLog()).not.toHaveTextContent('Настроить Go-окружение');
  });

  it('ошибка добавления задачи показывается в форме', async () => {
    backend(({ path }) => (path === '/api/v1/mcp/execute' ? json({ error: { code: 'INVALID_ARGUMENTS', message: 'дата уже прошла' } }, 422) : undefined));
    const user = userEvent.setup();
    renderApp();
    const form = await screen.findByRole('form', { name: 'Новая личная задача' });
    await user.type(within(form).getByLabelText('Название задачи'), 'Сдать зачёт');
    await user.click(within(form).getByRole('button', { name: 'Добавить задачу' }));
    expect(await within(form).findByRole('alert')).toHaveTextContent('дата уже прошла');
  });
});

describe('роли наставника и руководителя', () => {
  it('наставник видит бриф и уведомления, но не переписку; назначает обязательную задачу', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await screen.findByText('Чек-лист адаптации');
    await user.click(screen.getByRole('radio', { name: 'Наставник' }));

    expect(await screen.findByText(/Кабинет наставника · Мария Кузнецова/)).toBeInTheDocument();
    const brief = screen.getByTestId('brief');
    expect(within(brief).getByText(/Изучил инструкцию «Как проходить код-ревью»/)).toBeInTheDocument();
    expect(within(brief).getByText(/Отчитался по «Командный синк Autoteka»: Синк прошёл, блокеров нет/)).toBeInTheDocument();
    expect(within(brief).getByText(/mileage_verified/)).toBeInTheDocument();
    expect(screen.getAllByTestId('notification').length).toBeGreaterThanOrEqual(2);
    expect(screen.queryByLabelText('Сообщение Гоффи')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Спросить Гоффи' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Лента напоминаний')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Согласовать' })).not.toBeInTheDocument();
    expect(calls.find((c) => c.path === '/api/v1/events')?.body).toEqual({ name: 'ui.role_switched', payload: { role: 'mentor' } });

    const form = screen.getByRole('form', { name: 'Назначить обязательную задачу' });
    await user.type(within(form).getByLabelText('Название задачи'), 'Пройти курс ИБ');
    await user.click(within(form).getByRole('button', { name: 'Назначить' }));
    await waitFor(() =>
      expect(executeCalls(calls)[0]).toMatchObject({ token: 'demo-mentor-token', body: { tool: 'calendar.add_task', arguments: { title: 'Пройти курс ИБ', date: '2026-11-14' } } }),
    );
    expect(await screen.findByText('Обязательная задача «Пройти курс ИБ» назначена стажёру')).toBeInTheDocument();

    // Наставник видит карточку события, но отметить выполнение за стажёра не может.
    await user.click(screen.getAllByTestId('planner-item')[0]!);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByLabelText(/Отчёт для Гоффи/)).not.toBeInTheDocument();
  });

  it('пустой кабинет наставника предлагает запустить крон', async () => {
    const empty = { ...fixtures.workspaceMentor, briefs: [], notifications: [], absences: [] };
    const { calls } = backend(({ path, token }) => (path === '/api/v1/workspace' && token === 'demo-mentor-token' ? json(empty) : undefined));
    const user = userEvent.setup();
    useUi.getState().setRole('mentor');
    renderApp();
    expect(await screen.findByText('Брифов пока нет')).toBeInTheDocument();
    expect(screen.getByText('Новых уведомлений нет')).toBeInTheDocument();
    expect(screen.getByText('Заявок нет')).toBeInTheDocument();
    const main = screen.getByText('Брифов пока нет').closest('section')!;
    await user.click(within(main).getByRole('button', { name: 'Запустить ИИ-крон' }));
    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/cron/trigger')).toBe(true));
  });

  it('руководитель согласует отгул, видит инциденты и переключает runtime-флаги', async () => {
    const pending = structuredClone(fixtures.workspaceLead) as { absences: Array<{ status: string }> };
    pending.absences[0]!.status = 'pending_approval';
    const { calls } = backend(({ method, path, token }) =>
      method === 'GET' && path === '/api/v1/workspace' && token === 'demo-lead-token' ? json(pending) : undefined,
    );
    const user = userEvent.setup();
    useUi.getState().setRole('lead');
    renderApp();

    expect(await screen.findByText(/Кабинет руководителя · Дмитрий Орлов/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Согласовать' }));
    await waitFor(() =>
      expect(executeCalls(calls)[0]).toMatchObject({ token: 'demo-lead-token', body: { tool: 'absences.approve_day_off', arguments: { request_number: '#ABS-2026-84' } } }),
    );
    expect(await screen.findByText(/Отгул #ABS-2026-84 согласован/)).toBeInTheDocument();

    const incident = screen.getAllByTestId('incident')[0]!;
    expect(within(incident).getByText('db_admin.export_prod_pii')).toBeInTheDocument();

    const runtime = screen.getByRole('switch', { name: 'ai_buddy.absence_action_card' });
    expect(runtime).toBeChecked();
    await user.click(runtime);
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ path: '/api/v1/feature-flags/ai_buddy.absence_action_card', body: { enabled: false } }),
    );
    expect(screen.getByRole('switch', { name: 'ai_buddy.mcp_jsonrpc' })).toBeDisabled();
  });

  it('согласованный отгул показан без кнопки; ошибки согласования и флага видны', async () => {
    const pending = structuredClone(fixtures.workspaceLead) as { absences: Array<{ status: string }> };
    backend(({ method, path }) => {
      if (method === 'PUT' || path === '/api/v1/mcp/execute') return json(fixtures.errorForbidden, 403);
      return undefined;
    });
    const user = userEvent.setup();
    useUi.getState().setRole('lead');
    const view = renderApp();
    expect(await screen.findByText('Согласовано')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Согласовать' })).not.toBeInTheDocument();

    const flag = screen.getByRole('switch', { name: 'ai_buddy.absence_action_card' });
    await user.click(flag);
    expect(await screen.findByText(/default deny/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('switch', { name: 'ai_buddy.absence_action_card' })).toBeChecked());
    view.unmount();

    pending.absences[0]!.status = 'pending_approval';
    backend(({ method, path, token }) => {
      if (path === '/api/v1/mcp/execute') return json(fixtures.errorForbidden, 403);
      if (method === 'GET' && path === '/api/v1/workspace' && token === 'demo-lead-token') return json(pending);
      return undefined;
    });
    renderApp();
    await user.click(await screen.findByRole('button', { name: 'Согласовать' }));
    expect(await screen.findByTestId('toast')).toHaveTextContent(/default deny/);
  });
});

describe('тёмная тема', () => {
  it('переключается в шапке, применяется к документу и сохраняется', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    const toggle = await screen.findByRole('switch', { name: 'Тёмная тема' });
    expect(toggle).not.toBeChecked();
    expect(document.documentElement).not.toHaveClass('dark');

    await user.click(toggle);
    expect(toggle).toBeChecked();
    expect(document.documentElement).toHaveClass('dark');
    expect(JSON.parse(localStorage.getItem('ai-buddy-ui') ?? '{}').state.theme).toBe('dark');
    await waitFor(() => expect(calls.find((c) => c.path === '/api/v1/events')?.body).toEqual({ name: 'ui.theme_switched', payload: { theme: 'dark' } }));

    await user.click(toggle);
    expect(document.documentElement).not.toHaveClass('dark');
  });
});

describe('GoffyAvatar', () => {
  it('поддерживает размеры и пульсацию, по умолчанию рисует встроенного Гоффи', () => {
    const { container, rerender } = render(<GoffyAvatar />);
    expect(screen.getByRole('img', { name: 'Гоффи' })).toHaveAttribute('data-size', 'sm');
    expect(container.querySelector('svg')).toBeInTheDocument();
    expect(container.querySelector('.animate-wink')).toBeInTheDocument();
    expect(container.querySelector('.animate-pulse-ring')).not.toBeInTheDocument();
    rerender(<GoffyAvatar size="lg" pulse />);
    expect(screen.getByRole('img', { name: 'Гоффи' })).toHaveAttribute('data-size', 'lg');
    expect(container.querySelector('.animate-pulse-ring')).toBeInTheDocument();
    rerender(<GoffyAvatar size="md" />);
    expect(screen.getByRole('img', { name: 'Гоффи' })).toHaveAttribute('data-size', 'md');
  });

  it('подхватывает картинку маскота, если файл public/goffy.png существует', async () => {
    const probes: Array<{ onload?: () => void; onerror?: () => void; src?: string }> = [];
    vi.stubGlobal(
      'Image',
      class {
        onload?: () => void;
        onerror?: () => void;
        src?: string;
        constructor() {
          probes.push(this);
        }
      },
    );
    resetGoffyImageCache();
    const { container, unmount } = render(<GoffyAvatar size="md" />);
    expect(probes[0]?.src).toBe(GOFFY_IMAGE_URL);
    expect(container.querySelector('img')).not.toBeInTheDocument();
    act(() => probes[0]!.onload?.());
    expect(container.querySelector('img')).toHaveAttribute('src', GOFFY_IMAGE_URL);
    unmount();
    // Результат проверки кэшируется: следующий аватар сразу показывает картинку.
    expect(render(<GoffyAvatar />).container.querySelector('img')).toBeInTheDocument();

    resetGoffyImageCache();
    const missing = render(<GoffyAvatar />);
    act(() => probes[probes.length - 1]!.onerror?.());
    expect(missing.container.querySelector('svg')).toBeInTheDocument();
  });
});

describe('инспектор жюри', () => {
  it('показывает MCP/JSON-RPC, RBAC, clickstream, реестр и режим агента', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await screen.findByText('Чек-лист адаптации');
    await user.click(screen.getByRole('button', { name: /Инспектор жюри/ }));

    const drawer = await screen.findByRole('dialog', { name: 'Инспектор жюри' });
    expect(within(drawer).getByTestId('engine-info')).toHaveTextContent('Smart Engine');
    const calls = await within(drawer).findAllByTestId('tool-call');
    expect(calls.length).toBe(fixtures.telemetry.tool_calls.length);

    const forbidden = calls.find((c) => within(c).queryByText('db_admin.export_prod_pii'))!;
    expect(within(forbidden).getByText('запрещено')).toBeInTheDocument();
    await user.click(within(forbidden).getByRole('button'));
    expect(within(forbidden).getByText('FORBIDDEN (403)')).toBeInTheDocument();
    expect(within(forbidden).getByText(/политика POL-SEC-001/)).toBeInTheDocument();
    expect(within(forbidden).getByText('JSON-RPC · tools/call')).toBeInTheDocument();
    expect(forbidden).toHaveTextContent('"method": "tools/call"');
    expect(within(forbidden).queryByText('Результат')).not.toBeInTheDocument();

    const granted = calls.find((c) => within(c).queryByText('absences.create_day_off') && within(c).queryByText('успех'))!;
    await user.click(within(granted).getByRole('button'));
    expect(within(granted).getByText('Результат')).toBeInTheDocument();

    await user.click(within(drawer).getByRole('tab', { name: /RBAC/ }));
    expect(within(drawer).getByText('Матрица прав (default deny)')).toBeInTheDocument();
    expect(within(drawer).getAllByText('rbac.incident').length).toBeGreaterThanOrEqual(1);
    expect(within(drawer).getByText('calendar.complete_item', { selector: 'td' })).toBeInTheDocument();

    await user.click(within(drawer).getByRole('tab', { name: /Clickstream/ }));
    expect(within(drawer).getAllByText('agent.answer').length).toBeGreaterThanOrEqual(1);

    await user.click(within(drawer).getByRole('tab', { name: /Реестр/ }));
    expect(within(drawer).getAllByText('запрещён роли').length).toBeGreaterThanOrEqual(1);
    expect(within(drawer).getByText('с подтверждением')).toBeInTheDocument();

    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Инспектор жюри/ }));
    await user.click(await screen.findByRole('button', { name: 'Закрыть инспектор' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('пустое состояние и ошибка загрузки', async () => {
    let fail = false;
    const empty = { ...fixtures.telemetry, events: [], tool_calls: [] };
    backend(({ path }) => {
      if (path !== '/api/v1/telemetry') return undefined;
      return fail ? json({ error: { code: 'INTERNAL', message: 'Телеметрия недоступна' } }, 500) : json(empty);
    });
    const user = userEvent.setup();
    useUi.getState().openInspector('mcp');
    renderApp();
    const drawer = await screen.findByRole('dialog');
    expect(await within(drawer).findByText('Вызовов MCP пока нет')).toBeInTheDocument();
    await user.click(within(drawer).getByRole('tab', { name: /RBAC/ }));
    expect(within(drawer).getByText('Решений RBAC пока нет')).toBeInTheDocument();
    await user.click(within(drawer).getByRole('tab', { name: /Clickstream/ }));
    expect(within(drawer).getByText('Событий пока нет')).toBeInTheDocument();

    fail = true;
    act(() => {
      useUi.getState().closeInspector();
      useUi.getState().setRole('mentor'); // другой ключ запроса → новая загрузка
      useUi.getState().openInspector();
    });
    expect(await screen.findByText(/Не удалось загрузить телеметрию: Телеметрия недоступна/, {}, { timeout: 8000 })).toBeInTheDocument();
  }, 15000);
});
