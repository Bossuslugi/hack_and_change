// Серверное состояние через TanStack Query: запросы workspace/telemetry и мутации сценариев.
// Каждая мутация обновляет кэш из ответа сервера (без лишнего запроса) и инвалидирует зависимые данные.
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';
import { api, type DemoRole } from '../api/auth';
import { ApiError } from '../api/client';
import type { ChatMessage, ChatResult, CronResult, ToolCall, Workspace } from '../api/schemas';
import { useUi } from '../store/ui';

export const workspaceKey = (role: DemoRole) => ['workspace', role] as const;
export const telemetryKey = (role: DemoRole) => ['telemetry', role] as const;

export function errorText(error: unknown): string {
  if (error instanceof ApiError) return error.userMessage;
  return error instanceof Error ? error.message : 'Неизвестная ошибка';
}

export function useWorkspace() {
  const role = useUi((s) => s.role);
  return useQuery({
    queryKey: workspaceKey(role),
    queryFn: () => api.getWorkspace(),
    staleTime: 3_000,
    // Наставник и руководитель видят новые брифы и уведомления без перезагрузки.
    refetchInterval: 15_000,
    refetchOnWindowFocus: false,
    retry: 1,
  });
}

/** Телеметрия роли. С `live` опрашивается каждые 4 секунды (открытый инспектор), иначе обновляется по событиям. */
export function useTelemetry(live: boolean) {
  const role = useUi((s) => s.role);
  return useQuery({
    queryKey: telemetryKey(role),
    queryFn: () => api.getTelemetry(),
    staleTime: 3_000,
    refetchInterval: live ? 4_000 : false,
    refetchOnWindowFocus: false,
  });
}

/** MCP-вызовы сообщения по их id: сначала из ответа чата, после перезагрузки — из аудита. */
export function useToolCalls(ids: string[]): ToolCall[] {
  const local = useUi((s) => s.toolCalls);
  const audit = useTelemetry(false).data?.tool_calls;
  return useMemo(
    () => ids.map((id) => local[id] ?? audit?.find((call) => call.id === id)).filter((call): call is ToolCall => call !== undefined),
    [ids, local, audit],
  );
}

function upsertMessages(existing: ChatMessage[], incoming: Array<ChatMessage | null | undefined>): ChatMessage[] {
  const next = [...existing];
  for (const message of incoming) {
    if (!message) continue;
    const index = next.findIndex((m) => m.id === message.id);
    if (index === -1) next.push(message);
    else next[index] = message;
  }
  return next;
}

/** Применяет результат чата/действия к кэшу workspace и обновляет зависимые данные в фоне. */
export function applyChatResult(client: QueryClient, role: DemoRole, result: ChatResult, submittedMessageId?: string): void {
  useUi.getState().rememberToolCalls(result.tool_calls);
  client.setQueryData<Workspace>(workspaceKey(role), (ws) => {
    if (!ws) return ws;
    let messages = upsertMessages(ws.messages, [result.user_message, result.message]);
    if (submittedMessageId && result.message) {
      messages = messages.map((m) =>
        m.id === submittedMessageId && m.action_card ? { ...m, action_card: { ...m.action_card, status: 'submitted' as const } } : m,
      );
    }
    return {
      ...ws,
      messages,
      checklist: result.checklist.length > 0 ? result.checklist : ws.checklist,
      progress: result.progress,
      next_step: result.next_step ?? null,
    };
  });
  void client.invalidateQueries({ queryKey: ['workspace'] });
  void client.invalidateQueries({ queryKey: ['telemetry'] });
}

/** Отправка сообщения: оптимистичный показ, поток этапов и текста, обновление кэша. */
export function useSendMessage() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (text: string) => {
      const ui = useUi.getState();
      ui.beginSend(text);
      return api.streamChat(text, {
        onStage: (stage) => useUi.getState().pushStage(stage),
        onDelta: (delta) => useUi.getState().pushDelta(delta),
      });
    },
    onSuccess: (result) => {
      applyChatResult(client, useUi.getState().role, result);
      useUi.getState().endSend();
    },
    onError: (error, text) => {
      useUi.getState().failSend(errorText(error), text);
      void client.invalidateQueries({ queryKey: ['workspace'] });
    },
  });
}

export interface ExecuteInput {
  tool: string;
  args: Record<string, unknown>;
  messageId?: string;
}

/** Подтверждение интерактивной карточки → POST /api/v1/mcp/execute. */
export function useExecuteAction() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ tool, args, messageId }: ExecuteInput) => api.executeTool(tool, args, messageId),
    onSuccess: (result, input) => applyChatResult(client, useUi.getState().role, result, input.messageId),
    onError: () => void client.invalidateQueries({ queryKey: ['telemetry'] }),
  });
}

export function useCron() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => api.triggerCron(),
    onSuccess: (result: CronResult) => {
      const ui = useUi.getState();
      ui.setCronResult(result);
      ui.rememberToolCalls(result.tool_calls);
      ui.showToast({ kind: result.triggered && !result.deduplicated ? 'success' : 'info', text: `ИИ-крон: ${result.reason}` });
      void client.invalidateQueries({ queryKey: ['workspace'] });
      void client.invalidateQueries({ queryKey: ['telemetry'] });
    },
    onError: (error) => useUi.getState().showToast({ kind: 'error', text: errorText(error) }),
  });
}

export function useReset() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => api.reset(),
    onSuccess: (workspace) => {
      const ui = useUi.getState();
      ui.resetSession();
      client.removeQueries({ queryKey: ['workspace'] });
      client.setQueryData(workspaceKey(ui.role), workspace);
      void client.invalidateQueries({ queryKey: ['telemetry'] });
      ui.showToast({ kind: 'success', text: 'Демо-данные сброшены в исходное состояние' });
    },
    onError: (error) => useUi.getState().showToast({ kind: 'error', text: errorText(error) }),
  });
}

export function useSetFlag() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean }) => api.setFlag(key, enabled),
    // Оптимистичное переключение с откатом при ошибке.
    onMutate: async ({ key, enabled }) => {
      const queryKey = workspaceKey(useUi.getState().role);
      await client.cancelQueries({ queryKey });
      const previous = client.getQueryData<Workspace>(queryKey);
      client.setQueryData<Workspace>(queryKey, (ws) =>
        ws ? { ...ws, feature_flags: ws.feature_flags.map((f) => (f.key === key ? { ...f, enabled } : f)) } : ws,
      );
      return { previous, queryKey };
    },
    onError: (error, _input, context) => {
      if (context?.previous) client.setQueryData(context.queryKey, context.previous);
      useUi.getState().showToast({ kind: 'error', text: errorText(error) });
    },
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['workspace'] });
      void client.invalidateQueries({ queryKey: ['telemetry'] });
    },
  });
}
