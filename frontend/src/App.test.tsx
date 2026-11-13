import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import { setApiInternId, setApiRole } from './api/auth';
import { SCENARIOS } from './components/chat/ChatPanel';
import { useUi } from './store/ui';
import { chatStream, fakeFetch, fixtures, json, type Call, type Route } from './test/helpers';

type ChatFixture = typeof fixtures.chatAccess;

/** Имитация бэкенда с состоянием: ответы — реальные фикстуры API. */
function backend(overrides: Route = () => undefined) {
  let intern: Record<string, unknown> = structuredClone(fixtures.workspaceInitial);
  let lead: Record<string, unknown> = structuredClone(fixtures.workspaceLead);
  const byText: Record<string, unknown> = {
    [SCENARIOS[0]!.text]: fixtures.chatAccess,
    [SCENARIOS[1]!.text]: fixtures.chatPii,
    [SCENARIOS[2]!.text]: fixtures.chatProcess,
    [SCENARIOS[3]!.text]: fixtures.chatAbsence,
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
      if (token === 'demo-lead-token') return json(lead);
      return json(intern);
    }
    if (path === '/api/v1/chat/stream') {
      const result = (byText[(call.body as { message: string }).message] ?? fixtures.chatAccess) as ChatFixture;
      apply(result);
      return chatStream(result);
    }
    if (path === '/api/v1/mcp/execute') {
      apply(fixtures.executeAbsence as unknown as ChatFixture);
      return json(fixtures.executeAbsence);
    }
    if (path === '/api/v1/absences/review') {
      const b = (call.body ?? {}) as { id: string; decision: string; comment?: string };
      const current = ((lead.absences as any[]) ?? [])[0] ?? fixtures.workspaceLead.absences[0];
      const updated = {
        ...current,
        id: b.id || 'abs-a3afcc9043ac',
        status: b.decision || 'approved',
        decision: b.decision || 'approved',
        comment: b.comment,
      };
      lead.absences = ((lead.absences as any[]) ?? []).map((a: any) => (a.id === updated.id ? updated : a));
      return json(updated);
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

beforeEach(() => {
  // Фиксируем «сегодня»: форма отгула проверяет, что дата не в прошлом.
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(2026, 10, 13, 10, 0, 0));
  useUi.setState({ role: 'intern', activeInternId: 'u-intern-01', inspectorOpen: false, inspectorTab: 'mcp', toast: null });
  useUi.getState().resetSession();
  setApiRole('intern');
  setApiInternId('u-intern-01');
});

afterEach(() => {
  vi.useRealTimers();
});

describe('рабочее пространство стажёра', () => {
  it('показывает чек-лист, контекст, приветствие и дисклеймер синтетики', async () => {
    backend();
    renderApp();
    expect(screen.getByText(/Загружаю рабочее пространство/)).toBeInTheDocument();
    expect(await screen.findByText('Чек-лист адаптации')).toBeInTheDocument();
    expect(screen.getByText('Команда: Autoteka • Backend Go • Неделя 1 из 24')).toBeInTheDocument();
    expect(screen.getByText('Этап 1')).toBeInTheDocument();
    expect(screen.getByText('Доступы и окружение')).toBeInTheDocument();
    expect(within(step('access_repo')).getByText('GitHub / IAM')).toBeInTheDocument();
    expect(within(step('access_repo')).getByText('Ожидает')).toBeInTheDocument();
    expect(screen.getByText('Smart Engine', { selector: 'header *' })).toBeInTheDocument();
    expect(screen.getByText(/Синтетические данные: сотрудники/)).toBeInTheDocument();
    expect(step('access_repo')).toHaveAttribute('data-status', 'pending');
    expect(within(step('access_repo')).getByText('Следующий шаг')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '2');
    expect(screen.getByText(/Привет, Алексей/)).toBeInTheDocument();
    expect(screen.getByTestId('minutes-saved')).toHaveTextContent('0 мин');
  });

  it('сценарий 1А: доступ выдан, шаг №1 чек-листа выполнен', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: SCENARIOS[0]!.label }));

    const card = await screen.findByTestId('result-access_granted');
    expect(within(card).getByText('Доступ выдан')).toBeInTheDocument();
    expect(within(card).getByText('avito-autoteka-api')).toBeInTheDocument();
    expect(within(card).getByText(/ALLOWED · POL-IAM-001/)).toBeInTheDocument();
    await waitFor(() => expect(step('access_repo')).toHaveAttribute('data-status', 'completed'));
    expect(within(step('team_processes')).getByText('Следующий шаг')).toBeInTheDocument();
    expect(screen.getAllByTestId('user-message')[0]).toHaveTextContent(SCENARIOS[0]!.text);
    const plate = screen.getByTestId('mcp-plate');
    expect(plate).toHaveTextContent(/Вызван MCP: iam_access\.request_repo_access • RBAC: INTERN_ALLOWED/);
    await user.click(within(plate).getByRole('button'));
    expect(within(plate).getByText('Ответ сервиса')).toBeInTheDocument();
    expect(calls.find((c) => c.path === '/api/v1/chat/stream')?.body).toEqual({ message: SCENARIOS[0]!.text });
    await waitFor(() => expect(screen.queryByTestId('streaming')).not.toBeInTheDocument());
  });

  it('сценарий 1Б: RBAC блокирует запрос и предлагает песочницу', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: SCENARIOS[1]!.label }));

    const card = await screen.findByTestId('result-rbac_denied');
    expect(card).toHaveAttribute('role', 'alert');
    expect(within(card).getByText('Доступ запрещён · FORBIDDEN (403)')).toBeInTheDocument();
    expect(within(card).getByText('POL-SEC-001')).toBeInTheDocument();
    expect(within(card).getByText('зафиксирован в журнале безопасности')).toBeInTheDocument();
    expect(screen.getByText(/безопасную песочницу/)).toBeInTheDocument();
    expect(fixtures.chatPii.checklist_updates).toEqual([]); // отказ не двигает чек-лист
  });

  it('сценарий 2: источники помечены плашкой «Тестовая инструкция»', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: SCENARIOS[2]!.label }));

    const sources = await screen.findByLabelText('Источники');
    expect(within(sources).getAllByText('Тестовая инструкция (Синтетические данные)')).toHaveLength(2);
    expect(within(sources).getByText('Как катить задачу под фича-флагом')).toBeInTheDocument();
    await user.click(within(sources).getByRole('button', { name: /Как катить задачу под фича-флагом/ }));
    expect(within(sources).getByText(/github\.com\/avito-tech\/playbook/)).toBeInTheDocument();
    await waitFor(() => expect(step('team_processes')).toHaveAttribute('data-status', 'completed'));
  });

  it('сценарий 3: карточка отгула валидируется и отправляется через MCP', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: SCENARIOS[3]!.label }));

    const form = await screen.findByRole('form', { name: 'Заявка на отгул' });
    expect(within(form).getByText('Ждёт подтверждения')).toBeInTheDocument();
    expect(within(form).getByLabelText('Дата')).toHaveValue('2026-11-20');
    expect(within(form).getByLabelText('С')).toHaveValue('14:00');
    expect(within(form).getByLabelText('Причина')).toHaveValue('Зачёт в университете');

    // Клиентская валидация (Zod) до обращения к серверу.
    const reason = within(form).getByLabelText('Причина');
    await user.clear(reason);
    await user.click(within(form).getByRole('button', { name: 'Оформить отгул' }));
    expect(within(form).getByRole('alert')).toHaveTextContent('минимум 3 символа');
    expect(calls.some((c) => c.path === '/api/v1/mcp/execute')).toBe(false);

    await user.type(reason, 'Зачёт в университете');
    await user.click(within(form).getByLabelText('Руководителя'));
    await user.click(within(form).getByRole('button', { name: 'Оформить отгул' }));

    const done = await screen.findByTestId('result-absence_created');
    expect(within(done).getByText('#ABS-2026-84')).toBeInTheDocument();
    const execute = calls.find((c) => c.path === '/api/v1/mcp/execute');
    expect(execute?.body).toMatchObject({
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
    await user.click(await screen.findByRole('button', { name: SCENARIOS[3]!.label }));
    const form = await screen.findByRole('form', { name: 'Заявка на отгул' });
    await user.click(within(form).getByRole('button', { name: 'Оформить отгул' }));
    expect(await within(form).findByRole('alert')).toHaveTextContent(/default deny.*политика DEFAULT-DENY/);
    expect(within(form).getByRole('button', { name: 'Оформить отгул' })).toBeEnabled();
  });

  it('отправляет сообщение из поля ввода и по подсказке, проверяет пустой ввод', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    const input = await screen.findByLabelText('Сообщение AI-Buddy');

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

  it('кнопка «Запустить с AI-Buddy» у шага чек-листа запускает сценарий', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await screen.findByText('Чек-лист адаптации');
    await user.click(within(step('access_repo')).getByRole('button', { name: 'Запустить с AI-Buddy' }));
    await screen.findByTestId('result-access_granted');
    expect(calls.find((c) => c.path === '/api/v1/chat/stream')?.body).toEqual({ message: SCENARIOS[0]!.text });
  });

  it('при сбое показывает ошибку и даёт повторить', async () => {
    let fail = true;
    backend(({ path }) => {
      if (path === '/api/v1/chat/stream' && fail) return json({ error: { code: 'INTERNAL', message: 'Внутренняя ошибка сервера. Попробуйте ещё раз.' } }, 500);
      return undefined;
    });
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: SCENARIOS[0]!.label }));
    const alert = await screen.findByText(/Не удалось получить ответ: Внутренняя ошибка сервера/);
    expect(alert).toBeInTheDocument();

    fail = false;
    await user.click(screen.getByRole('button', { name: 'Повторить' }));
    await screen.findByTestId('result-access_granted');
    expect(screen.queryByText(/Не удалось получить ответ/)).not.toBeInTheDocument();
  });

  it('ошибку можно скрыть', async () => {
    backend(({ path }) => (path === '/api/v1/chat/stream' ? json({ error: { code: 'INTERNAL', message: 'сбой' } }, 500) : undefined));
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: SCENARIOS[0]!.label }));
    await user.click(await screen.findByRole('button', { name: 'Скрыть' }));
    expect(screen.queryByText(/Не удалось получить ответ/)).not.toBeInTheDocument();
  });

  it('запускает ИИ-крон и сбрасывает демо', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await user.click(await screen.findByRole('button', { name: 'Симулировать ИИ-крон' }));
    const statuses = await screen.findAllByText(/стажёру отправлено проактивное сообщение/);
    expect(statuses.length).toBeGreaterThanOrEqual(1);
    await user.click(screen.getByRole('button', { name: 'Скрыть уведомление' }));

    await user.click(screen.getByRole('button', { name: 'Сбросить демо' }));
    expect(await screen.findByText('Демо-данные сброшены в исходное состояние')).toBeInTheDocument();
    expect(calls.some((c) => c.method === 'POST' && c.path === '/api/v1/reset')).toBe(true);
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

describe('роли наставника и руководителя', () => {
  it('наставник видит бриф и уведомления, но не переписку', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    renderApp();
    await screen.findByText('Чек-лист адаптации');
    await user.click(screen.getByRole('radio', { name: 'Наставник' }));

    expect(await screen.findByText(/Кабинет наставника · Мария Кузнецова/)).toBeInTheDocument();
    const brief = screen.getByTestId('brief');
    expect(within(brief).getByText(/Изучил инструкцию «Как проходить код-ревью»/)).toBeInTheDocument();
    expect(within(brief).getByText(/mileage_verified/)).toBeInTheDocument();
    expect(screen.getAllByTestId('notification').length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText('#ABS-2026-84', { selector: 'td' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Сообщение AI-Buddy')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Запустить с AI-Buddy' })).not.toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Наставник' })).toBeChecked();
    expect(calls.some((c) => c.path === '/api/v1/workspace' && c.token === 'demo-mentor-token')).toBe(true);
    expect(calls.find((c) => c.path === '/api/v1/events')?.body).toEqual({ name: 'ui.role_switched', payload: { role: 'mentor' } });
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
    await user.click(within(main).getByRole('button', { name: 'Симулировать ИИ-крон' }));
    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/cron/trigger')).toBe(true));
  });

  it('руководитель видит инциденты и переключает runtime-флаги', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    useUi.getState().setRole('lead');
    renderApp();

    expect(await screen.findByText(/Кабинет руководителя · Дмитрий Орлов/)).toBeInTheDocument();
    const incident = screen.getAllByTestId('incident')[0]!;
    expect(within(incident).getByText('db_admin.export_prod_pii')).toBeInTheDocument();
    expect(within(incident).getByText(/POL-SEC-001/)).toBeInTheDocument();

    const runtime = screen.getByRole('switch', { name: 'ai_buddy.absence_action_card' });
    expect(runtime).toBeChecked();
    await user.click(runtime);
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ path: '/api/v1/feature-flags/ai_buddy.absence_action_card', body: { enabled: false }, token: 'demo-lead-token' }),
    );
    expect(screen.getByRole('switch', { name: 'ai_buddy.mcp_jsonrpc' })).toBeDisabled();
  });

  it('откатывает флаг и показывает ошибку, если сервер отказал', async () => {
    backend(({ method }) => (method === 'PUT' ? json(fixtures.errorForbidden, 403) : undefined));
    const user = userEvent.setup();
    useUi.getState().setRole('lead');
    renderApp();
    const flag = await screen.findByRole('switch', { name: 'ai_buddy.absence_action_card' });
    await user.click(flag);
    expect(await screen.findByText(/default deny/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('switch', { name: 'ai_buddy.absence_action_card' })).toBeChecked());
  });

  it('руководитель согласует заявку на отгул стажёра (двусторонний цикл)', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    useUi.getState().setRole('lead');
    renderApp();

    expect(await screen.findByText(/Кабинет руководителя/)).toBeInTheDocument();
    const row = screen.getByTestId('absence-row-abs-a3afcc9043ac');
    expect(within(row).getByText('Ожидает согласования')).toBeInTheDocument();

    const approveBtn = within(row).getByRole('button', { name: /Согласовать отгул/ });
    expect(approveBtn).toBeInTheDocument();
    await user.click(approveBtn);

    await waitFor(() => {
      expect(within(row).getByText('Согласовано руководителем')).toBeInTheDocument();
    });
    expect(calls.some((c) => c.path === '/api/v1/absences/review')).toBe(true);
  });

  it('руководитель отклоняет заявку на отгул стажёра', async () => {
    const { calls } = backend();
    const user = userEvent.setup();
    useUi.getState().setRole('lead');
    renderApp();

    expect(await screen.findByText(/Кабинет руководителя/)).toBeInTheDocument();
    const row = screen.getByTestId('absence-row-abs-a3afcc9043ac');
    const rejectBtn = within(row).getByRole('button', { name: /Отклонить отгул/ });
    await user.click(rejectBtn);

    await waitFor(() => {
      expect(within(row).getByText('Отклонено')).toBeInTheDocument();
    });
    expect(calls.some((c) => c.path === '/api/v1/absences/review')).toBe(true);
  });

  it('переключает профиль стажёра в шапке и адаптирует контекст под ML', async () => {
    let activeInternId = 'u-intern-01';
    const mariaWorkspace = {
      ...fixtures.workspaceInitial,
      intern: {
        id: 'u-intern-02',
        name: 'Мария Козлова',
        role: 'intern_backend',
        team: 'Авито Товары (Модерация)',
        stack: 'Python/ML',
        stage: 'Week 3',
        program: 'Стажировка ML, 6 месяцев',
        start_date: '2026-10-26',
        repository: 'avito-moderation-ml',
        mentor_id: 'u-mentor-02',
        mentor_name: 'Елена Соколова',
        team_lead_id: 'u-lead-01',
        team_lead_name: 'Дмитрий Орлов',
        is_synthetic: true,
      },
    };
    backend((call) => {
      if (call.path === '/api/v1/workspace') {
        if (call.internId === 'u-intern-02' || call.url?.includes('intern_id=u-intern-02') || activeInternId === 'u-intern-02') {
          return json(mariaWorkspace);
        }
      }
      return undefined;
    });
    const user = userEvent.setup();
    renderApp();
    await screen.findByText('Чек-лист адаптации');

    activeInternId = 'u-intern-02';
    // Кликаем по переключателю профиля стажёра "Мария"
    const mariaBtn = screen.getByRole('radio', { name: /Мария/ });
    expect(mariaBtn).toBeInTheDocument();
    await user.click(mariaBtn);

    // Контекст в шапке обновляется
    expect(await screen.findByLabelText('Контекст стажёра')).toHaveTextContent(/Авито Товары \(Модерация\)/);
    expect(screen.getByLabelText('Контекст стажёра')).toHaveTextContent(/Python\/ML/);

    // Специфичные ML сценарии в чате
    expect(await screen.findByText('ML-пайплайны: Airflow & DVC')).toBeInTheDocument();
    expect(screen.getByText('Катить ML-модель & A/B-тест')).toBeInTheDocument();
  });

  it('шаг чек-листа имеет микроанимационные классы перехода', async () => {
    backend();
    renderApp();
    await screen.findByText('Чек-лист адаптации');
    const step1 = screen.getByTestId('step-access_repo');
    expect(step1.className).toContain('transition-all');
    expect(step1.className).toContain('duration-500');
  });
});

describe('инспектор телеметрии', () => {
  it('показывает MCP-вызовы, RBAC, clickstream и реестр', async () => {
    backend();
    const user = userEvent.setup();
    renderApp();
    await screen.findByText('Чек-лист адаптации');
    await user.click(screen.getByRole('button', { name: /Инспектор/ }));

    const drawer = await screen.findByRole('dialog', { name: 'Инспектор телеметрии' });
    const calls = await within(drawer).findAllByTestId('tool-call');
    expect(calls.length).toBe(fixtures.telemetry.tool_calls.length);

    const forbidden = calls.find((c) => within(c).queryByText('db_admin.export_prod_pii'))!;
    expect(within(forbidden).getByText('запрещено')).toBeInTheDocument();
    await user.click(within(forbidden).getByRole('button'));
    expect(within(forbidden).getByText('FORBIDDEN (403)')).toBeInTheDocument();
    expect(within(forbidden).getByText('Аргументы')).toBeInTheDocument();
    expect(within(forbidden).queryByText('Результат')).not.toBeInTheDocument();

    const granted = calls.find((c) => within(c).queryByText('absences.create_day_off') && within(c).queryByText('успех'))!;
    await user.click(within(granted).getByRole('button'));
    expect(within(granted).getByText('Результат')).toBeInTheDocument();

    await user.click(within(drawer).getByRole('tab', { name: /RBAC/ }));
    expect(within(drawer).getByText('Матрица прав (default deny)')).toBeInTheDocument();
    expect(within(drawer).getAllByText('rbac.incident').length).toBeGreaterThanOrEqual(1);
    expect(within(drawer).getByText('absences.create_day_off', { selector: 'td' })).toBeInTheDocument();

    await user.click(within(drawer).getByRole('tab', { name: /Clickstream/ }));
    expect(within(drawer).getAllByText('agent.answer').length).toBeGreaterThanOrEqual(1);

    await user.click(within(drawer).getByRole('tab', { name: /Реестр/ }));
    expect(within(drawer).getAllByText('запрещён роли').length).toBeGreaterThanOrEqual(1);
    expect(within(drawer).getByText('с подтверждением')).toBeInTheDocument();

    await user.click(within(drawer).getByRole('tab', { name: /SDD/ }));
    expect(within(drawer).getByText('spec_id: avito-start-ai-buddy-rbac')).toBeInTheDocument();
    expect(within(drawer).getByText('ai_buddy.proactive_cron')).toBeInTheDocument();

    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Инспектор/ }));
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
    useUi.getState().closeInspector();
    useUi.getState().setRole('mentor'); // другой ключ запроса → новая загрузка
    useUi.getState().openInspector();
    expect(await screen.findByText(/Не удалось загрузить телеметрию: Телеметрия недоступна/, {}, { timeout: 8000 })).toBeInTheDocument();
  }, 15000);
});
