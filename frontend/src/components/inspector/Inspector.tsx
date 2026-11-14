import { useEffect, useState } from 'react';
import type { Telemetry, TelemetryEvent, ToolCall, Workspace } from '../../api/schemas';
import { errorText, useTelemetry } from '../../hooks/useBuddy';
import { formatTime, TOOL_STATUS_LABEL } from '../../lib/format';
import { useUi, type InspectorTab } from '../../store/ui';
import { Badge, Button, EmptyState, JsonBlock, Spinner, cn } from '../ui/primitives';

const TABS: Array<{ id: InspectorTab; label: string }> = [
  { id: 'mcp', label: 'MCP' },
  { id: 'rbac', label: 'RBAC' },
  { id: 'clickstream', label: 'Clickstream' },
  { id: 'registry', label: 'Реестр' },
];

const STATUS_TONE = { success: 'ok', forbidden: 'danger', invalid: 'warn', error: 'danger' } as const;
const SEVERITY_TONE = { info: 'neutral', warn: 'warn', security: 'danger' } as const;

function ToolCallRow({ call }: { call: ToolCall }) {
  const [open, setOpen] = useState(false);
  return (
    <li className="rounded-xl border border-paper-200 bg-paper-0" data-testid="tool-call">
      <button type="button" onClick={() => setOpen((v) => !v)} aria-expanded={open} className="flex w-full items-center gap-2 px-3 py-2.5 text-left">
        <Badge tone={STATUS_TONE[call.status]}>{TOOL_STATUS_LABEL[call.status]}</Badge>
        <span className="min-w-0 flex-1 truncate font-mono text-xs font-medium text-ink-900">{call.tool}</span>
        <span className="shrink-0 text-[11px] text-ink-400">
          {call.actor_role} · {call.duration_ms.toFixed(1)} мс · {formatTime(call.created_at)}
        </span>
      </button>
      {open && (
        <div className="space-y-2.5 border-t border-paper-100 px-3 py-3">
          <p className="text-xs text-ink-700">
            RBAC:{' '}
            <span className={cn('font-semibold', call.rbac.allowed ? 'text-ok-700' : 'text-danger-700')}>
              {call.rbac.code}
              {!call.rbac.allowed && ` (${call.rbac.http_status})`}
            </span>{' '}
            · политика {call.rbac.policy_id}
            {call.rbac.constraint && ` · ограничение ${call.rbac.constraint}`}
          </p>
          <p className="text-xs text-ink-500">{call.rbac.reason}</p>
          {call.error && <p className="text-xs text-danger-700">{call.error_code}: {call.error}</p>}
          <JsonBlock
            label="JSON-RPC · tools/call"
            value={{ jsonrpc: '2.0', method: 'tools/call', params: { name: call.tool, arguments: call.arguments } }}
          />
          {call.status === 'success' && <JsonBlock label="Результат" value={call.result} />}
        </div>
      )}
    </li>
  );
}

function EventRow({ event }: { event: TelemetryEvent }) {
  const summary = Object.entries(event.payload)
    .filter(([, v]) => typeof v !== 'object' && v !== '')
    .slice(0, 5)
    .map(([k, v]) => `${k}=${String(v)}`)
    .join(' · ');
  return (
    <li className="rounded-xl border border-paper-200 bg-paper-0 px-3 py-2" data-testid="event">
      <div className="flex items-center gap-2">
        <Badge tone={SEVERITY_TONE[event.severity]}>{event.severity}</Badge>
        <span className="min-w-0 flex-1 truncate font-mono text-xs font-medium text-ink-900">{event.name}</span>
        <span className="shrink-0 text-[11px] text-ink-400">
          {event.actor_role} · {formatTime(event.created_at)}
        </span>
      </div>
      {summary && <p className="mt-1 break-words font-mono text-[11px] leading-relaxed text-ink-500">{summary}</p>}
    </li>
  );
}

function Matrix({ data }: { data: Telemetry }) {
  return (
    <div className="overflow-x-auto rounded-xl border border-paper-200 bg-paper-0">
      <table className="w-full text-left text-[11px]">
        <thead className="bg-paper-50 text-ink-500">
          <tr>
            <th className="px-2.5 py-2 font-medium">Операция</th>
            {data.roles.map((r) => (
              <th key={r.id} className="px-1.5 py-2 text-center font-medium" title={r.title}>
                {r.id.replace('_backend', '').replace('system_', '')}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-paper-100">
          {data.matrix.map((row) => (
            <tr key={row.operation}>
              <td className="px-2.5 py-1.5 font-mono text-ink-900">{row.operation}</td>
              {data.roles.map((r) => (
                <td key={r.id} className="px-1.5 py-1.5 text-center">
                  {row.access[r.id] ? (
                    <span className="font-semibold text-ok-700" aria-label="разрешено">✓</span>
                  ) : (
                    <span className="text-ink-300" aria-label="запрещено">—</span>
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function TabContent({ tab, data }: { tab: InspectorTab; data: Telemetry }) {
  if (tab === 'mcp') {
    return data.tool_calls.length === 0 ? (
      <EmptyState title="Вызовов MCP пока нет" hint="Запусти любой сценарий в чате — здесь появится аудит вызовов инструментов." />
    ) : (
      <ul className="space-y-2">
        {data.tool_calls.map((c) => (
          <ToolCallRow key={c.id} call={c} />
        ))}
      </ul>
    );
  }
  if (tab === 'rbac') {
    const decisions = data.events.filter((e) => e.kind === 'rbac');
    return (
      <div className="space-y-4">
        <div>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-400">Решения и инциденты</h3>
          {decisions.length === 0 ? (
            <EmptyState title="Решений RBAC пока нет" />
          ) : (
            <ul className="space-y-2">
              {decisions.map((e) => (
                <EventRow key={e.id} event={e} />
              ))}
            </ul>
          )}
        </div>
        <div>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-400">Матрица прав (default deny)</h3>
          <Matrix data={data} />
        </div>
      </div>
    );
  }
  if (tab === 'clickstream') {
    const events = data.events.filter((e) => e.kind !== 'rbac' && e.kind !== 'mcp');
    return events.length === 0 ? (
      <EmptyState title="Событий пока нет" />
    ) : (
      <ul className="space-y-2">
        {events.map((e) => (
          <EventRow key={e.id} event={e} />
        ))}
      </ul>
    );
  }
  return (
    <ul className="space-y-2">
      {data.tools.map((t) => (
        <li key={t.name} className="rounded-xl border border-paper-200 bg-paper-0 px-3 py-2.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs font-medium text-ink-900">{t.name}</span>
            <Badge tone={t.allowed_for_viewer ? 'ok' : 'danger'}>{t.allowed_for_viewer ? 'доступен роли' : 'запрещён роли'}</Badge>
            {t.requires_confirmation && <Badge tone="brand">с подтверждением</Badge>}
            <Badge>{t.side_effects}</Badge>
          </div>
          <p className="mt-1 text-xs leading-relaxed text-ink-500">{t.description}</p>
        </li>
      ))}
    </ul>
  );
}

/** Выдвижной инспектор телеметрии: аудит MCP, решения RBAC, Clickstream и реестр инструментов. */
export function Inspector({ workspace }: { workspace?: Workspace }) {
  const open = useUi((s) => s.inspectorOpen);
  const tab = useUi((s) => s.inspectorTab);
  const setTab = useUi((s) => s.setInspectorTab);
  const close = useUi((s) => s.closeInspector);
  const query = useTelemetry(open);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, close]);

  if (!open) return null;
  const data = query.data;
  const counts: Record<InspectorTab, number | undefined> = {
    mcp: data?.tool_calls.length,
    rbac: data?.events.filter((e) => e.kind === 'rbac').length,
    clickstream: data?.events.filter((e) => e.kind !== 'rbac' && e.kind !== 'mcp').length,
    registry: data?.tools.length,
  };

  return (
    <aside
      role="dialog"
      aria-label="Инспектор жюри"
      className="fixed inset-y-0 right-0 z-30 flex w-full max-w-[520px] flex-col border-l border-paper-200 bg-paper-50 shadow-drawer"
    >
      <div className="flex items-center justify-between border-b border-paper-200 bg-paper-0 px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold text-ink-900">Инспектор жюри</h2>
          <p className="text-[11px] text-ink-500">Техническая подноготная: MCP / JSON-RPC, решения RBAC, Clickstream. Стажёр этого не видит</p>
        </div>
        <Button variant="ghost" size="sm" onClick={close} aria-label="Закрыть инспектор">
          Закрыть
        </Button>
      </div>
      {workspace && (
        <p className="border-b border-paper-200 bg-paper-0 px-4 py-2 text-[11px] text-ink-500" data-testid="engine-info">
          Режим агента: <span className="font-medium text-ink-900">{workspace.engine.mode === 'llm' ? `LLM · ${workspace.engine.model}` : 'Smart Engine'}</span> ·
          хранилище: {workspace.storage.store} · кэш: {workspace.storage.cache}
        </p>
      )}
      <div role="tablist" aria-label="Разделы инспектора" className="flex gap-1 border-b border-paper-200 bg-paper-0 px-3 py-2">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              'rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500',
              tab === t.id ? 'bg-brand-50 text-brand-700' : 'text-ink-500 hover:bg-paper-100',
            )}
          >
            {t.label}
            {counts[t.id] !== undefined && <span className="ml-1 text-ink-400">{counts[t.id]}</span>}
          </button>
        ))}
      </div>
      <div className="flex-1 overflow-y-auto p-3" role="tabpanel">
        {query.isLoading && (
          <div className="flex items-center gap-2 p-4 text-sm text-ink-500">
            <Spinner /> Загружаю телеметрию…
          </div>
        )}
        {query.isError && (
          <div role="alert" className="rounded-xl bg-danger-50 p-3 text-sm text-danger-700">
            Не удалось загрузить телеметрию: {errorText(query.error)}
            <Button size="sm" variant="danger" className="ml-2" onClick={() => void query.refetch()}>
              Повторить
            </Button>
          </div>
        )}
        {data && <TabContent tab={tab} data={data} />}
      </div>
    </aside>
  );
}
