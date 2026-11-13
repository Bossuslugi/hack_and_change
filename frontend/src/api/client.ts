// Выделенный API-клиент AI-Buddy. Единственная точка общения интерфейса с бэкендом:
// добавляет демо-токен роли, валидирует каждый ответ Zod-схемой и приводит ошибки к ApiError.
import type { z } from 'zod';
import {
  apiErrorSchema,
  chatResultSchema,
  cronResultSchema,
  featureFlagSchema,
  stageSchema,
  streamErrorSchema,
  telemetrySchema,
  workspaceSchema,
  type ChatResult,
  type CronResult,
  type FeatureFlag,
  type RbacVerdict,
  type Stage,
  type Telemetry,
  type Workspace,
} from './schemas';
import { readSse } from './sse';

/** Ошибка API с машинным кодом и, для отказов RBAC, вердиктом политики. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly rbac?: RbacVerdict;

  constructor(status: number, code: string, message: string, rbac?: RbacVerdict | null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.rbac = rbac ?? undefined;
  }

  /** Понятное пользователю сообщение. */
  get userMessage(): string {
    switch (this.code) {
      case 'NETWORK':
        return 'Нет связи с сервером. Проверь, что бэкенд запущен, и повтори попытку.';
      case 'SCHEMA_MISMATCH':
        return 'Сервер вернул ответ в неожиданном формате. Обнови страницу или повтори позже.';
      case 'UNAUTHORIZED':
        return 'Сессия не авторизована. Выбери роль заново.';
      default:
        return this.message;
    }
  }
}

export interface ApiClientOptions {
  /** Базовый URL API; пустая строка — тот же origin. */
  baseUrl?: string;
  /** Возвращает демо-токен текущей роли. */
  getToken: () => string;
  fetch?: typeof fetch;
}

export interface StreamHandlers {
  onStage?: (stage: Stage) => void;
  onDelta?: (text: string) => void;
}

export interface ApiClient {
  getWorkspace(): Promise<Workspace>;
  sendChat(message: string): Promise<ChatResult>;
  streamChat(message: string, handlers?: StreamHandlers): Promise<ChatResult>;
  executeTool(tool: string, args: Record<string, unknown>, messageId?: string): Promise<ChatResult>;
  triggerCron(): Promise<CronResult>;
  reset(): Promise<Workspace>;
  getTelemetry(): Promise<Telemetry>;
  setFlag(key: string, enabled: boolean): Promise<FeatureFlag>;
  trackEvent(name: string, payload?: Record<string, string | number | boolean>): Promise<void>;
}

function parse<S extends z.ZodTypeAny>(schema: S, data: unknown, status: number): z.output<S> {
  const result = schema.safeParse(data);
  if (!result.success) {
    const issue = result.error.issues[0];
    const where = issue ? `${issue.path.join('.')}: ${issue.message}` : 'неизвестное поле';
    throw new ApiError(status, 'SCHEMA_MISMATCH', `Ответ сервера не соответствует контракту (${where})`);
  }
  return result.data;
}

async function toApiError(response: Response): Promise<ApiError> {
  let body: unknown = null;
  try {
    body = await response.json();
  } catch {
    // тело не JSON — используем статус
  }
  const parsed = apiErrorSchema.safeParse(body);
  if (parsed.success) {
    const { code, message, rbac } = parsed.data.error;
    return new ApiError(response.status, code, message, rbac);
  }
  return new ApiError(response.status, `HTTP_${response.status}`, `Сервер ответил ошибкой ${response.status}`);
}

export function createApiClient(options: ApiClientOptions): ApiClient {
  const baseUrl = (options.baseUrl ?? '').replace(/\/+$/, '');
  const doFetch: typeof fetch = (input, init) => (options.fetch ?? globalThis.fetch)(input, init);

  async function send(method: string, path: string, body?: unknown): Promise<Response> {
    const headers: Record<string, string> = { Authorization: `Bearer ${options.getToken()}` };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    try {
      return await doFetch(`${baseUrl}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (cause) {
      throw new ApiError(0, 'NETWORK', cause instanceof Error ? cause.message : 'Сеть недоступна');
    }
  }

  async function request<S extends z.ZodTypeAny>(schema: S, method: string, path: string, body?: unknown): Promise<z.output<S>> {
    const response = await send(method, path, body);
    if (!response.ok) throw await toApiError(response);
    let data: unknown;
    try {
      data = await response.json();
    } catch {
      throw new ApiError(response.status, 'SCHEMA_MISMATCH', 'Ответ сервера не является JSON');
    }
    return parse(schema, data, response.status);
  }

  const sendChat = (message: string) => request(chatResultSchema, 'POST', '/api/v1/chat', { message });

  return {
    getWorkspace: () => request(workspaceSchema, 'GET', '/api/v1/workspace'),
    sendChat,

    // Потоковый чат (SSE поверх fetch): этапы обработки и фрагменты текста приходят по мере готовности.
    // Если поток недоступен (старый прокси, сеть), клиент прозрачно откатывается на обычный POST /chat.
    async streamChat(message, handlers = {}) {
      let response: Response;
      try {
        response = await send('POST', '/api/v1/chat/stream', { message });
      } catch {
        return sendChat(message);
      }
      if (!response.ok) throw await toApiError(response);
      if (!response.body || !(response.headers.get('Content-Type') ?? '').includes('text/event-stream')) {
        return sendChat(message);
      }

      let result: ChatResult | undefined;
      let failure: ApiError | undefined;
      try {
        await readSse(response.body, (event, data) => {
          if (event === 'stage') {
            const stage = stageSchema.safeParse(data);
            if (stage.success) handlers.onStage?.(stage.data);
          } else if (event === 'delta') {
            const text = (data as { text?: unknown } | null)?.text;
            if (typeof text === 'string') handlers.onDelta?.(text);
          } else if (event === 'done') {
            result = parse(chatResultSchema, data, 200);
          } else if (event === 'error') {
            const parsed = streamErrorSchema.safeParse(data);
            failure = parsed.success
              ? new ApiError(parsed.data.status, parsed.data.error.code, parsed.data.error.message, parsed.data.error.rbac)
              : new ApiError(500, 'INTERNAL', 'Сервер прервал ответ');
          }
        });
      } catch (cause) {
        if (cause instanceof ApiError) throw cause;
        throw new ApiError(0, 'NETWORK', 'Соединение прервалось во время ответа');
      }
      if (failure) throw failure;
      if (!result) throw new ApiError(0, 'NETWORK', 'Сервер закрыл соединение, не завершив ответ');
      return result;
    },

    executeTool: (tool, args, messageId) =>
      request(chatResultSchema, 'POST', '/api/v1/mcp/execute', { tool, arguments: args, message_id: messageId ?? '' }),
    triggerCron: () => request(cronResultSchema, 'POST', '/api/v1/cron/trigger'),
    reset: () => request(workspaceSchema, 'POST', '/api/v1/reset'),
    getTelemetry: () => request(telemetrySchema, 'GET', '/api/v1/telemetry'),
    setFlag: (key, enabled) => request(featureFlagSchema, 'PUT', `/api/v1/feature-flags/${encodeURIComponent(key)}`, { enabled }),

    // Clickstream — best-effort: сбой аналитики не должен мешать пользователю.
    async trackEvent(name, payload = {}) {
      try {
        await send('POST', '/api/v1/events', { name, payload });
      } catch {
        // игнорируем
      }
    },
  };
}
