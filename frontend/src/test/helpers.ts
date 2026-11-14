// Фикстуры — реальные ответы бэкенда (сняты скриптом с запущенного сервера),
// поэтому тесты схем и клиента одновременно являются контрактными тестами API.
import chatAbsence from './fixtures/chat_absence.json';
import chatAccess from './fixtures/chat_access.json';
import chatPii from './fixtures/chat_pii.json';
import chatProcess from './fixtures/chat_process.json';
import cron from './fixtures/cron.json';
import errorForbidden from './fixtures/error_forbidden.json';
import executeAbsence from './fixtures/execute_absence.json';
import executeAddTask from './fixtures/execute_add_task.json';
import executeApprove from './fixtures/execute_approve.json';
import executeAssignTask from './fixtures/execute_assign_task.json';
import executeReport from './fixtures/execute_report.json';
import executeDuplicate from './fixtures/execute_duplicate.json';
import executeForbidden from './fixtures/execute_forbidden.json';
import flag from './fixtures/flag.json';
import telemetry from './fixtures/telemetry.json';
import workspaceInitial from './fixtures/workspace_initial.json';
import workspaceIntern from './fixtures/workspace_intern.json';
import workspaceLead from './fixtures/workspace_lead.json';
import workspaceMentor from './fixtures/workspace_mentor.json';

export const fixtures = {
  chatAbsence,
  chatAccess,
  chatPii,
  chatProcess,
  cron,
  errorForbidden,
  executeAbsence,
  executeAddTask,
  executeApprove,
  executeAssignTask,
  executeReport,
  executeDuplicate,
  executeForbidden,
  flag,
  telemetry,
  workspaceInitial,
  workspaceIntern,
  workspaceLead,
  workspaceMentor,
};

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

/** SSE-ответ из списка событий; chunkSize режет поток на куски, как это делает сеть. */
export function sse(events: Array<[string, unknown]>, chunkSize = 37): Response {
  const text = events.map(([event, data]) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`).join('');
  const bytes = new TextEncoder().encode(text);
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (let i = 0; i < bytes.length; i += chunkSize) controller.enqueue(bytes.slice(i, i + chunkSize));
      controller.close();
    },
  });
  return new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream; charset=utf-8' } });
}

/** Поток чата: этапы → фрагменты текста → итог. */
export function chatStream(result: { message: { text: string } | null }): Response {
  const text = result.message?.text ?? '';
  return sse([
    ['stage', { kind: 'understanding', label: 'Разбираюсь в запросе…' }],
    ['stage', { kind: 'rbac', label: 'Проверяю права роли intern_backend…', tool: 'iam_access.request_repo_access' }],
    ['stage', { kind: 'tool', label: 'Запрашиваю доступ в IAM…', tool: 'iam_access.request_repo_access' }],
    ['delta', { text: text.slice(0, 20) }],
    ['delta', { text: text.slice(20) }],
    ['done', result],
  ]);
}

export interface Call {
  method: string;
  path: string;
  token: string;
  body: unknown;
}

export type Route = (call: Call) => Response | Promise<Response> | undefined;

/** Подменяет fetch маршрутизатором и запоминает вызовы. */
export function fakeFetch(route: Route) {
  const calls: Call[] = [];
  const fn = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = new URL(String(input), 'http://localhost');
    const headers = new Headers(init?.headers);
    const call: Call = {
      method: init?.method ?? 'GET',
      path: url.pathname,
      token: (headers.get('Authorization') ?? '').replace('Bearer ', ''),
      body: typeof init?.body === 'string' ? JSON.parse(init.body) : undefined,
    };
    calls.push(call);
    const response = await route(call);
    return response ?? json({ error: { code: 'NOT_FOUND', message: `нет маршрута ${call.method} ${call.path}` } }, 404);
  };
  return { fn: fn as typeof fetch, calls };
}
