import { describe, expect, it, vi } from 'vitest';
import { chatStream, fakeFetch, fixtures, json, sse } from '../test/helpers';
import { getApiInternId, setApiInternId } from './auth';
import { ApiError, createApiClient } from './client';
import {
  chatResultSchema,
  cronResultSchema,
  featureFlagSchema,
  telemetrySchema,
  workspaceSchema,
} from './schemas';
import { dispatchSseBlock, readSse } from './sse';

describe('контракт API: схемы принимают реальные ответы бэкенда', () => {
  it.each([
    ['workspace (исходное состояние)', workspaceSchema, fixtures.workspaceInitial],
    ['workspace стажёра', workspaceSchema, fixtures.workspaceIntern],
    ['workspace наставника', workspaceSchema, fixtures.workspaceMentor],
    ['workspace руководителя', workspaceSchema, fixtures.workspaceLead],
    ['чат: доступ', chatResultSchema, fixtures.chatAccess],
    ['чат: блокировка RBAC', chatResultSchema, fixtures.chatPii],
    ['чат: процессы', chatResultSchema, fixtures.chatProcess],
    ['чат: отгул', chatResultSchema, fixtures.chatAbsence],
    ['execute: отгул', chatResultSchema, fixtures.executeAbsence],
    ['execute: личная задача', chatResultSchema, fixtures.executeAddTask],
    ['execute: обязательная задача', chatResultSchema, fixtures.executeAssignTask],
    ['execute: отчёт Гоффи', chatResultSchema, fixtures.executeReport],
    ['execute: согласование отгула', chatResultSchema, fixtures.executeApprove],
    ['крон', cronResultSchema, fixtures.cron],
    ['телеметрия', telemetrySchema, fixtures.telemetry],
    ['флаг', featureFlagSchema, fixtures.flag],
  ] as const)('%s', (_name, schema, fixture) => {
    const result = schema.safeParse(fixture);
    expect(result.success ? [] : result.error.issues).toEqual([]);
  });

  it('каждый источник содержит флаг синтетики и дисклеймер', () => {
    const result = chatResultSchema.parse(fixtures.chatProcess);
    expect(result.message?.sources).toHaveLength(2);
    for (const source of result.message?.sources ?? []) {
      expect(source.is_synthetic).toBe(true);
      expect(source.disclaimer).toBe('Тестовая инструкция (Синтетические данные)');
    }
    const broken = structuredClone(fixtures.chatProcess) as { message: { sources: Array<Record<string, unknown>> } };
    delete broken.message.sources[0]!.disclaimer;
    expect(chatResultSchema.safeParse(broken).success).toBe(false);
  });

  it('null-коллекции нормализуются в пустые', () => {
    const raw = structuredClone(fixtures.workspaceInitial) as Record<string, unknown>;
    raw.briefs = null;
    raw.messages = null;
    const ws = workspaceSchema.parse(raw);
    expect(ws.briefs).toEqual([]);
    expect(ws.messages).toEqual([]);
  });

  it('workspace содержит календарь, напоминания и статистику стажёра', () => {
    const ws = workspaceSchema.parse(fixtures.workspaceInitial);
    expect(ws.planner).toHaveLength(6);
    expect(ws.planner.find((p) => p.id === 'plan-1on1')).toMatchObject({ kind: 'meeting', mandatory: true, status: 'planned', is_synthetic: true });
    expect(ws.planner.find((p) => p.id === 'plan-1on1')?.link).toMatch(/^https:\/\/meet\.avito\.example\//);
    expect(ws.reminders[0]).toMatchObject({ urgent: true, item_id: 'plan-1on1', text: '🚨 Через 3 часа — Созвон 1:1 с наставником' });
    expect(ws.metrics).toMatchObject({ deflection_rate: 0, access_grants: 0, tasks_done: 0, tasks_total: 6 });
    const after = workspaceSchema.parse(fixtures.workspaceIntern);
    expect(after.reminders.some((r) => r.text.startsWith('🔔 Руководитель согласовал твой отгул #ABS-2026-84'))).toBe(true);
    expect(after.metrics.access_grants).toBe(1);
    expect(after.metrics.deflection_rate).toBe(100);
  });

  it('служебные детали не попадают в текст и карточки чата', () => {
    for (const fixture of [fixtures.chatAccess, fixtures.chatPii, fixtures.chatProcess, fixtures.chatAbsence, fixtures.executeAbsence]) {
      const message = chatResultSchema.parse(fixture).message!;
      const visible = [message.text, message.result_card?.title, ...(message.result_card?.lines ?? []).flatMap((l) => [l.label, l.value])].join(' ');
      expect(visible).not.toMatch(/POL-|FORBIDDEN|ALLOWED|RBAC|iam_access|db_admin|playbook\.|absences\.|intern_backend/);
    }
  });

  it('отказ RBAC несёт вердикт FORBIDDEN (403)', () => {
    const result = chatResultSchema.parse(fixtures.chatPii);
    expect(result.rbac).toMatchObject({ allowed: false, code: 'FORBIDDEN', http_status: 403, policy_id: 'POL-SEC-001', incident: true });
    expect(result.message?.result_card?.type).toBe('rbac_denied');
  });
});

describe('SSE', () => {
  it('разбирает блоки и пропускает мусор', () => {
    const handler = vi.fn();
    dispatchSseBlock('event: stage\r\ndata: {"kind":"rbac"}', handler);
    dispatchSseBlock('data: {"a":1}', handler);
    dispatchSseBlock('event: delta\ndata: {не json', handler);
    dispatchSseBlock(': комментарий', handler);
    expect(handler.mock.calls).toEqual([
      ['stage', { kind: 'rbac' }],
      ['message', { a: 1 }],
    ]);
  });

  it('склеивает события, разрезанные на произвольные куски, включая кириллицу', async () => {
    const events: Array<[string, unknown]> = [
      ['delta', { text: 'Привет, ' }],
      ['delta', { text: 'Алексей!' }],
      ['done', { ok: true }],
    ];
    for (const chunk of [1, 3, 7, 1000]) {
      const seen: Array<[string, unknown]> = [];
      await readSse(sse(events, chunk).body!, (event, data) => seen.push([event, data]));
      expect(seen).toEqual(events);
    }
  });

  it('обрабатывает последний блок без завершающей пустой строки', async () => {
    const bytes = new TextEncoder().encode('event: done\ndata: {"ok":true}');
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(bytes);
        c.close();
      },
    });
    const handler = vi.fn();
    await readSse(stream, handler);
    expect(handler).toHaveBeenCalledWith('done', { ok: true });
  });
});

function client(route: Parameters<typeof fakeFetch>[0], token = 'demo-intern-token') {
  const fake = fakeFetch(route);
  return { api: createApiClient({ baseUrl: 'http://api.test/', getToken: () => token, fetch: fake.fn }), calls: fake.calls };
}

describe('API-клиент', () => {
  it('добавляет токен роли и валидирует ответ', async () => {
    const { api, calls } = client(() => json(fixtures.workspaceInitial));
    const ws = await api.getWorkspace();
    expect(ws.intern.team).toBe('Autoteka');
    expect(calls[0]).toMatchObject({ method: 'GET', path: '/api/v1/workspace', token: 'demo-intern-token' });
  });

  it('по умолчанию использует тот же origin и глобальный fetch', async () => {
    const spy = vi.fn(async () => json(fixtures.flag));
    vi.stubGlobal('fetch', spy);
    const api = createApiClient({ getToken: () => 't' });
    await api.setFlag('ai_buddy.proactive_cron', false);
    expect(spy).toHaveBeenCalledWith('/api/v1/feature-flags/ai_buddy.proactive_cron', expect.objectContaining({ method: 'PUT' }));
  });

  it('отправляет тела запросов по контракту', async () => {
    const { api, calls } = client(({ path }) => {
      if (path === '/api/v1/chat') return json(fixtures.chatAccess);
      if (path === '/api/v1/mcp/execute') return json(fixtures.executeAbsence);
      if (path === '/api/v1/cron/trigger') return json(fixtures.cron);
      if (path === '/api/v1/reset') return json(fixtures.workspaceInitial);
      if (path === '/api/v1/telemetry') return json(fixtures.telemetry);
      if (path === '/api/v1/events') return json({ id: 'evt-1' }, 202);
      return undefined;
    });
    expect((await api.sendChat('привет')).message?.intent).toBe('access_request');
    const done = await api.executeTool('absences.create_day_off', { date: '2026-11-20' }, 'msg-1');
    expect(done.tool_calls[0]?.result.request_number).toBe('#ABS-2026-84');
    await api.executeTool('feature_flags.list', {});
    expect((await api.triggerCron()).brief?.architecture_question).toContain('mileage_verified');
    expect((await api.reset()).messages).toHaveLength(1);
    expect((await api.getTelemetry()).tools).toHaveLength(11);
    await api.trackEvent('ui.role_switched', { role: 'mentor' });
    await api.trackEvent('ui.opened');

    expect(calls.map((c) => c.body)).toEqual([
      { message: 'привет' },
      { tool: 'absences.create_day_off', arguments: { date: '2026-11-20' }, message_id: 'msg-1' },
      { tool: 'feature_flags.list', arguments: {}, message_id: '' },
      undefined,
      undefined,
      undefined,
      { name: 'ui.role_switched', payload: { role: 'mentor' } },
      { name: 'ui.opened', payload: {} },
    ]);
  });

  it('приводит ошибки сервера к ApiError с вердиктом RBAC', async () => {
    const { api } = client(() => json(fixtures.errorForbidden, 403));
    const error = await api.sendChat('привет').catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 403, code: 'FORBIDDEN', rbac: { operation: 'api.chat.send', allowed: false } });
    expect((error as ApiError).userMessage).toContain('default deny');

    const dup = client(() => json(fixtures.executeDuplicate, 409));
    await expect(dup.api.executeTool('absences.create_day_off', {})).rejects.toMatchObject({ status: 409, code: 'DUPLICATE' });
  });

  it('обрабатывает не-JSON ошибки, сеть и нарушение контракта', async () => {
    const html = client(() => new Response('<html>502</html>', { status: 502 }));
    await expect(html.api.getWorkspace()).rejects.toMatchObject({ status: 502, code: 'HTTP_502' });

    const down = client(() => {
      throw new TypeError('Failed to fetch');
    });
    const netError = (await down.api.getWorkspace().catch((e: unknown) => e)) as ApiError;
    expect(netError).toMatchObject({ status: 0, code: 'NETWORK' });
    expect(netError.userMessage).toContain('Нет связи');
    await expect(down.api.trackEvent('ui.x')).resolves.toBeUndefined(); // clickstream не мешает пользователю

    const drift = client(() => json({ ...fixtures.workspaceInitial, progress: 'много' }));
    const schemaError = (await drift.api.getWorkspace().catch((e: unknown) => e)) as ApiError;
    expect(schemaError.code).toBe('SCHEMA_MISMATCH');
    expect(schemaError.message).toContain('progress');
    expect(schemaError.userMessage).toContain('неожиданном формате');

    const notJson = client(() => new Response('ok', { status: 200 }));
    await expect(notJson.api.getWorkspace()).rejects.toMatchObject({ code: 'SCHEMA_MISMATCH' });

    expect(new ApiError(401, 'UNAUTHORIZED', 'x').userMessage).toContain('роль');
    expect(new ApiError(0, 'NETWORK', 'x', null).rbac).toBeUndefined();
  });

  it('reviewAbsence отправляет решение руководителя и возвращает обновлённую заявку', async () => {
    const fakeAbsence = {
      id: 'abs-1',
      number: '#ABS-2026-84',
      intern_id: 'u-intern-01',
      intern_name: 'Алексей Смирнов',
      date: '2026-11-20',
      time_from: '14:00',
      time_to: '18:00',
      reason: 'Зачёт в университете',
      status: 'approved',
      decision: 'approved',
      notified: ['u-mentor-01', 'u-lead-01'],
      is_synthetic: true,
      created_at: '2026-11-13T10:00:00Z',
    };
    const { api, calls } = client(({ path }) => {
      if (path === '/api/v1/absences/review') return json(fakeAbsence);
      return undefined;
    });
    const res = await api.reviewAbsence('abs-1', 'approved', 'Согласовано');
    expect(res.status).toBe('approved');
    expect(res.number).toBe('#ABS-2026-84');
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/absences/review',
      body: { id: 'abs-1', decision: 'approved', comment: 'Согласовано' },
    });
  });

  it('передаёт X-Intern-ID и query-параметр при наличии getInternId', async () => {
    const fake = fakeFetch(() => json(fixtures.workspaceInitial));
    const customApi = createApiClient({
      baseUrl: 'http://api.test/',
      getToken: () => 'lead-token',
      getInternId: () => 'u-intern-02',
      fetch: fake.fn,
    });
    await customApi.getWorkspace();
    expect(fake.calls[0]?.url).toBe('/api/v1/workspace?intern_id=u-intern-02');
    expect(fake.calls[0]?.internId).toBe('u-intern-02');
  });

  it('управляет активным ID стажёра в auth', () => {
    setApiInternId('u-intern-02');
    expect(getApiInternId()).toBe('u-intern-02');
    setApiInternId('u-intern-01');
    expect(getApiInternId()).toBe('u-intern-01');
  });
});

describe('потоковый чат', () => {
  it('передаёт этапы и текст, возвращает итог', async () => {
    const { api, calls } = client(() => chatStream(fixtures.chatAccess));
    const stages: string[] = [];
    let text = '';
    const result = await api.streamChat('Дай доступ', {
      onStage: (s) => stages.push(s.kind),
      onDelta: (t) => {
        text += t;
      },
    });
    expect(stages).toEqual(['understanding', 'rbac', 'tool']);
    expect(text).toBe(fixtures.chatAccess.message.text);
    expect(result.message?.result_card?.type).toBe('access_granted');
    expect(calls[0]).toMatchObject({ path: '/api/v1/chat/stream', body: { message: 'Дай доступ' } });
    // Обработчики необязательны.
    await expect(client(() => chatStream(fixtures.chatAccess)).api.streamChat('x')).resolves.toBeTruthy();
  });

  it('откатывается на обычный POST /chat, если поток недоступен', async () => {
    const noStream = client(({ path }) => (path.endsWith('/stream') ? json(fixtures.chatAccess) : json(fixtures.chatProcess)));
    expect((await noStream.api.streamChat('x')).message?.intent).toBe('process_question');
    expect(noStream.calls.map((c) => c.path)).toEqual(['/api/v1/chat/stream', '/api/v1/chat']);

    const flaky = client(({ path }) => {
      if (path.endsWith('/stream')) throw new TypeError('network');
      return json(fixtures.chatAccess);
    });
    expect((await flaky.api.streamChat('x')).message?.intent).toBe('access_request');
  });

  it('превращает ошибки потока в ApiError', async () => {
    const before = client(() => json(fixtures.errorForbidden, 403));
    await expect(before.api.streamChat('x')).rejects.toMatchObject({ status: 403, code: 'FORBIDDEN' });

    const inside = client(() => sse([['error', { status: 403, error: fixtures.errorForbidden.error }]]));
    await expect(inside.api.streamChat('x')).rejects.toMatchObject({ status: 403, rbac: { policy_id: 'DEFAULT-DENY' } });

    const garbage = client(() => sse([['error', { oops: true }]]));
    await expect(garbage.api.streamChat('x')).rejects.toMatchObject({ status: 500, code: 'INTERNAL' });

    const cut = client(() => sse([['stage', { kind: 'rbac', label: 'Проверяю…' }], ['stage', { bad: 1 }], ['delta', { text: 5 }]]));
    await expect(cut.api.streamChat('x')).rejects.toMatchObject({ code: 'NETWORK' });

    const drift = client(() => sse([['done', { message: 'не объект' }]]));
    await expect(drift.api.streamChat('x')).rejects.toMatchObject({ code: 'SCHEMA_MISMATCH' });

    const broken = client(() => {
      const stream = new ReadableStream<Uint8Array>({
        start(c) {
          c.error(new Error('reset'));
        },
      });
      return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } });
    });
    await expect(broken.api.streamChat('x')).rejects.toMatchObject({ code: 'NETWORK' });
  });
});
