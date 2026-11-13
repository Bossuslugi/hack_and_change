import { useEffect, useState } from 'react';
import type { Telemetry, TelemetryEvent, ToolCall } from '../../api/schemas';
import { errorText, useTelemetry, useWorkspace } from '../../hooks/useBuddy';
import { formatMs, formatTime, TOOL_STATUS_LABEL } from '../../lib/format';
import { useUi, type InspectorTab } from '../../store/ui';
import { Icon } from '../ui/icons';
import { Badge, Button, EmptyState, Eyebrow, FOCUS, JsonBlock, Skeleton, cn } from '../ui/primitives';

const TABS: Array<{ id: InspectorTab; label: string }> = [
  { id: 'mcp', label: 'MCP' },
  { id: 'rbac', label: 'RBAC' },
  { id: 'clickstream', label: 'Clickstream' },
  { id: 'registry', label: 'Реестр' },
  { id: 'sdd', label: 'SDD' },
];

/** Спецификации из /specs, по которым собраны агент, шлюз MCP и матрица прав. */
const SPECS: Array<{ id: string; file: string; scope: string }> = [
  { id: 'avito-start-ai-buddy', file: '01_agent_behavior.openspec.yaml', scope: 'Промпт, интенты, эффекты чек-листа' },
  { id: 'avito-start-ai-buddy-mcp', file: '02_mcp_tools.openspec.yaml', scope: 'Контракты и JSON Schema инструментов' },
  { id: 'avito-start-ai-buddy-rbac', file: '03_rbac_matrix.openspec.yaml', scope: 'Роли, политики, ограничения' },
];

const STATUS_TONE = { success: 'ok', forbidden: 'danger', invalid: 'warn', error: 'danger' } as const;
const SEVERITY_TONE = { info: 'neutral', warn: 'warn', security: 'danger' } as const;

const cardClass = 'rounded-card border border-line bg-white';

function ToolCallRow({ call }: { call: ToolCall }) {
  const [open, setOpen] = useState(false);
  return (
    <li className={cardClass} data-testid="tool-call">
      <button type="button" onClick={() => setOpen((v) => !v)} aria-expanded={open} className={cn('w-full rounded-card p-2 text-left', FOCUS)}>
        <span className="flex items-center gap-2">
          <Badge tone={STATUS_TONE[call.status]}>{TOOL_STATUS_LABEL[call.status]}</Badge>
          <span className="min-w-0 flex-1 truncate font-mono text-meta font-medium text-ink-900">{call.tool}</span>
          <Icon name="chevron" className={cn('h-3 w-3 text-ink-400 transition-transform', open && 'rotate-180')} />
        </span>
        <span className="mt-1 block font-mono text-meta text-ink-400">
          {call.actor_role} · {formatMs(call.duration_ms)} · {formatTime(call.created_at)}
        </span>
      </button>
      {open && (
        <div className="space-y-2 border-t border-line p-2">
          <div className="space-y-1 font-mono text-meta text-ink-500">
            <p>user_role: {call.rbac.role}</p>
            <p>requested_scope: {call.rbac.operation}</p>
            <p>
              verdict:{' '}
              <span className={cn('font-medium', call.rbac.allowed ? 'text-ok-700' : 'text-danger-700')}>
                {call.rbac.code}
                {!call.rbac.allowed && ` (${call.rbac.http_status})`}
              </span>
            </p>
            <p>
              policy: {call.rbac.policy_id}
              {call.rbac.constraint && ` · constraint: ${call.rbac.constraint}`}
            </p>
          </div>
          <p className="text-meta text-ink-500">{call.rbac.reason}</p>
          {call.error && (
            <p className="text-meta text-danger-700">
              {call.error_code}: {call.error}
            </p>
          )}
          <JsonBlock label="Аргументы" value={call.arguments} />
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
    <li className={cn(cardClass, 'p-2')} data-testid="event">
      <div className="flex items-center gap-2">
        <Badge mono tone={SEVERITY_TONE[event.severity]}>
          {event.severity}
        </Badge>
        <span className="min-w-0 flex-1 truncate font-mono text-meta font-medium text-ink-900">{event.name}</span>
        <span className="shrink-0 font-mono text-meta text-ink-400">{formatTime(event.created_at)}</span>
      </div>
      {summary && <p className="mt-1 break-words font-mono text-meta text-ink-500">{summary}</p>}
    </li>
  );
}

function Matrix({ data }: { data: Telemetry }) {
  return (
    <div className={cn(cardClass, 'scroll-thin overflow-x-auto')}>
      <table className="w-full text-left text-meta">
        <thead className="bg-canvas text-ink-500">
          <tr>
            <th className="p-2 font-medium">Операция</th>
            {data.roles.map((r) => (
              <th key={r.id} className="p-2 text-center font-mono font-normal" title={r.title}>
                {r.id.replace('_backend', '').replace('system_', '')}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {data.matrix.map((row) => (
            <tr key={row.operation}>
              <td className="p-2 font-mono text-ink-900">{row.operation}</td>
              {data.roles.map((r) => (
                <td key={r.id} className="p-2 text-center">
                  {row.access[r.id] ? (
                    <span className="font-semibold text-ok-700" aria-label="разрешено">
                      ✓
                    </span>
                  ) : (
                    <span className="text-ink-400" aria-label="запрещено">
                      —
                    </span>
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

/** Спецификации и активные флаги: чем управляется поведение агента в эту минуту. */
function SpecsAndFlags() {
  const ws = useWorkspace().data;
  const flags = ws?.feature_flags ?? [];
  const skills = ws?.skills ?? [];
  const runtimeFlags = flags.filter((f) => f.kind === 'runtime');
  const compileFlags = flags.filter((f) => f.kind === 'compile_time');

  return (
    <div className="space-y-4">
      {skills.length > 0 && (
        <div>
          <Eyebrow className="mb-2">Модульные скиллы агента (Skills)</Eyebrow>
          <ul className="space-y-2">
            {skills.map((skill) => (
              <li key={skill.id} className={cn(cardClass, 'p-2')}>
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-meta font-medium text-ink-900">{skill.id}</span>
                  <Badge tone={skill.active ? 'ok' : 'neutral'}>{skill.active ? 'active' : 'disabled'}</Badge>
                </div>
                <p className="mt-1 text-meta font-medium text-ink-800">{skill.name}</p>
                <p className="mt-0.5 text-meta text-ink-500">{skill.description}</p>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div>
        <Eyebrow className="mb-2">Применённые спецификации</Eyebrow>
        <ul className="space-y-2">
          {SPECS.map((spec) => (
            <li key={spec.id} className={cn(cardClass, 'p-2')}>
              <p className="font-mono text-meta font-medium text-ink-900">spec_id: {spec.id}</p>
              <p className="mt-1 text-meta text-ink-500">{spec.scope}</p>
              <p className="mt-1 font-mono text-meta text-ink-400">specs/{spec.file}</p>
            </li>
          ))}
        </ul>
      </div>
      <div>
        <Eyebrow className="mb-2">Feature flags</Eyebrow>
        <div className="space-y-3">
          <div>
            <p className="mb-1 text-meta font-medium text-ink-700">Runtime Flags (Redis / на лету)</p>
            <ul className={cn(cardClass, 'divide-y divide-line')}>
              {runtimeFlags.map((flag) => (
                <li key={flag.key} className="flex items-center justify-between gap-2 p-2 font-mono text-meta" title={flag.description}>
                  <div className="min-w-0">
                    <span className="block truncate text-ink-900">{flag.key}</span>
                    <span className="text-meta text-ink-400 font-sans">{flag.description}</span>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <Badge mono>runtime</Badge>
                    <span className={cn('font-medium', flag.enabled ? 'text-ok-700' : 'text-ink-400')}>{String(flag.enabled)}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
          <div>
            <p className="mb-1 text-meta font-medium text-ink-700">Compile-Time Flags (embed / сборка)</p>
            <ul className={cn(cardClass, 'divide-y divide-line')}>
              {compileFlags.map((flag) => (
                <li key={flag.key} className="flex items-center justify-between gap-2 p-2 font-mono text-meta" title={flag.description}>
                  <div className="min-w-0">
                    <span className="block truncate text-ink-900">{flag.key}</span>
                    <span className="text-meta text-ink-400 font-sans">{flag.description}</span>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <Badge mono tone="neutral">compile-time</Badge>
                    <span className={cn('font-medium', flag.enabled ? 'text-ok-700' : 'text-ink-400')}>{String(flag.enabled)}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
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
          <Eyebrow className="mb-2">Решения и инциденты</Eyebrow>
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
          <h3 className="mb-2 font-mono text-meta uppercase tracking-wider text-ink-400">Матрица прав (default deny)</h3>
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
  if (tab === 'sdd') return <SpecsAndFlags />;
  return (
    <ul className="space-y-2">
      {data.tools.map((t) => (
        <li key={t.name} className={cn(cardClass, 'p-2')}>
          <p className="font-mono text-meta font-medium text-ink-900">{t.name}</p>
          <div className="mt-1 flex flex-wrap items-center gap-1">
            <Badge tone={t.allowed_for_viewer ? 'ok' : 'danger'}>{t.allowed_for_viewer ? 'доступен роли' : 'запрещён роли'}</Badge>
            {t.requires_confirmation && <Badge tone="accent">с подтверждением</Badge>}
            <Badge mono>{t.side_effects}</Badge>
          </div>
          <p className="mt-1 text-meta text-ink-500">{t.description}</p>
        </li>
      ))}
    </ul>
  );
}

/** Правый дровер «Под капотом»: аудит MCP, решения RBAC, Clickstream, реестр инструментов, спецификации и флаги. */
export function Inspector() {
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
    sdd: undefined,
  };

  return (
    <aside role="dialog" aria-label="Инспектор телеметрии" className="glass flex min-h-0 w-[360px] shrink-0 flex-col overflow-hidden rounded-panel shadow-float">
      <div className="flex h-14 shrink-0 items-center justify-between gap-2 border-b border-line px-4">
        <div className="min-w-0">
          <h2 className="text-body font-semibold text-ink-900">Под капотом</h2>
          <p className="truncate font-mono text-meta text-ink-400">live · обновление каждые 4 с</p>
        </div>
        <Button variant="ghost" size="sm" className="w-8 px-0" onClick={close} aria-label="Закрыть инспектор">
          <Icon name="close" />
        </Button>
      </div>
      <div role="tablist" aria-label="Разделы инспектора" className="scroll-thin flex shrink-0 overflow-x-auto border-b border-line p-2">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              'inline-flex h-8 items-center gap-1 rounded-tag px-2 text-meta font-medium transition-colors',
              FOCUS,
              tab === t.id ? 'bg-ink-900 text-white' : 'text-ink-500 hover:bg-ink-900/5',
            )}
          >
            {t.label}
            {counts[t.id] !== undefined && <span className="font-mono font-normal opacity-60">{counts[t.id]}</span>}
          </button>
        ))}
      </div>
      <div className="scroll-thin min-h-0 flex-1 overflow-y-auto p-2" role="tabpanel">
        {query.isLoading && (
          <div className="space-y-2" role="status" aria-label="Загружаю телеметрию">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        )}
        {query.isError && !data && (
          <div role="alert" className="rounded-card bg-danger-50 p-2 text-meta text-danger-700">
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
