import { useQueryClient } from '@tanstack/react-query';
import { api, DEMO_INTERNS, DEMO_ROLES, DEMO_USERS, type DemoRole } from '../api/auth';
import type { Workspace } from '../api/schemas';
import { useCron, useReset, useTelemetry } from '../hooks/useBuddy';
import { internContext } from '../lib/format';
import { useUi } from '../store/ui';
import { Icon, type IconName } from './ui/icons';
import { Badge, Button, FOCUS, StatusDot, cn, type Tone } from './ui/primitives';

const ROLE_ICON: Record<DemoRole, IconName> = { intern: 'user', mentor: 'mentor', lead: 'chart' };

/** Четыре круга палитры: знак продукта. */
function BrandMark() {
  return (
    <svg viewBox="0 0 32 32" className="h-8 w-8 shrink-0" role="img" aria-label="Avito Start">
      <circle cx="10" cy="10" r="6" fill="#00AAFF" />
      <circle cx="22" cy="10" r="6" fill="#00C257" />
      <circle cx="10" cy="22" r="6" fill="#8B5CF6" />
      <circle cx="22" cy="22" r="6" fill="#FF3B30" />
    </svg>
  );
}

/** Индикатор у вкладки роли: что ждёт этой роли (готовый бриф, заявка на согласование). */
function roleSignal(role: DemoRole, workspace?: Workspace): { tone: Tone; hint: string } | null {
  if (!workspace) return null;
  if (role === 'mentor' && workspace.metrics.briefs_prepared > 0) return { tone: 'ok', hint: 'бриф к 1:1 готов' };
  if (role === 'lead') {
    const pending = workspace.absences.filter((a) => a.status === 'pending_approval').length;
    if (pending > 0) return { tone: 'warn', hint: `заявок на согласование: ${pending}` };
  }
  return null;
}

function RoleSwitcher({ workspace }: { workspace?: Workspace }) {
  const role = useUi((s) => s.role);
  const setRole = useUi((s) => s.setRole);

  const choose = (next: DemoRole) => {
    if (next === role) return;
    setRole(next);
    void api.trackEvent('ui.role_switched', { role: next });
  };

  return (
    <div role="radiogroup" aria-label="Роль пользователя" className="inline-flex items-center gap-1 rounded-card bg-ink-900/5 p-1">
      {DEMO_ROLES.map((r) => {
        const user = DEMO_USERS[r];
        const active = role === r;
        const signal = roleSignal(r, workspace);
        return (
          <button
            key={r}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={user.label}
            title={signal ? `${user.label}: ${user.name} · ${signal.hint}` : `${user.label}: ${user.name}`}
            onClick={() => choose(r)}
            className={cn(
              'inline-flex h-8 items-center gap-2 whitespace-nowrap rounded-tag px-3 text-meta font-semibold transition-all',
              FOCUS,
              active ? 'bg-ink-900 text-white shadow-card' : 'text-ink-500 hover:bg-white/70 hover:text-ink-900',
            )}
          >
            <Icon name={ROLE_ICON[r]} className="h-3.5 w-3.5" />
            {user.label}
            {signal && <StatusDot tone={signal.tone} pulse={!active} />}
          </button>
        );
      })}
    </div>
  );
}

/** Профиль стажёра: переключает демо-данные между Go- и ML-стажёром. */
function InternSwitcher() {
  const activeInternId = useUi((s) => s.activeInternId);
  const setActiveInternId = useUi((s) => s.setActiveInternId);
  const queryClient = useQueryClient();

  const choose = (id: string) => {
    if (id === activeInternId) return;
    setActiveInternId(id);
    void api.trackEvent('ui.intern_switched', { intern_id: id });
    void queryClient.invalidateQueries({ queryKey: ['workspace'] });
    void queryClient.invalidateQueries({ queryKey: ['telemetry'] });
  };

  return (
    <div role="radiogroup" aria-label="Профиль стажёра" className="inline-flex shrink-0 items-center gap-1 rounded-card bg-ink-900/5 p-1">
      {DEMO_INTERNS.map((intern) => {
        const active = activeInternId === intern.id;
        return (
          <button
            key={intern.id}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={`${intern.name} (${intern.team})`}
            title={`${intern.name}: ${intern.roleTitle} • ${intern.team} • ${intern.stage}`}
            onClick={() => choose(intern.id)}
            className={cn(
              'inline-flex h-8 items-center gap-2 whitespace-nowrap rounded-tag px-2 text-meta font-medium transition-all',
              FOCUS,
              active ? 'bg-white text-ink-900 shadow-card' : 'text-ink-500 hover:text-ink-900',
            )}
          >
            <span className={cn('h-2 w-2 rounded-full', intern.stack === 'Go' ? 'bg-accent-500' : 'bg-ai-500')} aria-hidden="true" />
            {intern.name.split(' ')[0]}
            <span className="font-mono font-normal text-ink-400">{intern.stack === 'Go' ? 'Go' : 'ML'}</span>
          </button>
        );
      })}
    </div>
  );
}

export function Header({ workspace }: { workspace?: Workspace }) {
  const inspectorOpen = useUi((s) => s.inspectorOpen);
  const reset = useReset();
  const cron = useCron();
  const events = useTelemetry(false).data?.events.length;

  const toggleInspector = () => {
    const ui = useUi.getState();
    if (inspectorOpen) return ui.closeInspector();
    ui.openInspector();
    void api.trackEvent('ui.inspector_opened', {});
  };

  return (
    <header className="glass relative z-30 grid min-h-16 shrink-0 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-6 border-x-0 border-t-0 border-b-line px-5 shadow-card">
      <div className="flex min-w-0 items-center gap-3">
        <BrandMark />
        <div className="flex shrink-0 items-center gap-2">
          <span className="whitespace-nowrap text-body font-extrabold tracking-tight text-ink-900">Avito Start</span>
          <Badge tone="ai" mono className="whitespace-nowrap">
            AI-Buddy
          </Badge>
        </div>
        <InternSwitcher />
        {workspace && (
          <Badge className="hidden min-w-0 max-w-full 2xl:inline-flex" title={`${workspace.intern.name} · ${workspace.intern.program}`}>
            <span className="truncate whitespace-nowrap" aria-label="Контекст стажёра">
              {internContext(workspace.intern)}
            </span>
          </Badge>
        )}
      </div>

      <RoleSwitcher workspace={workspace} />

      <div className="flex min-w-0 items-center justify-end gap-2">
        {workspace && (
          <Badge
            mono
            tone={workspace.engine.mode === 'llm' ? 'ok' : 'neutral'}
            className="hidden whitespace-nowrap 2xl:inline-flex"
            title={`Хранилище: ${workspace.storage.store} · кэш: ${workspace.storage.cache}`}
          >
            <StatusDot tone="ok" />
            {workspace.engine.mode === 'llm' ? `LLM · ${workspace.engine.model}` : 'Smart Engine'}
          </Badge>
        )}
        {workspace?.permissions.cron && (
          <Button
            variant="ai"
            size="compact"
            onClick={() => cron.mutate()}
            loading={cron.isPending}
            aria-label="Симулировать ИИ-крон"
            title="Проверить застрявшие задачи стажёра и собрать наставнику бриф к 1:1"
          >
            {!cron.isPending && <Icon name="bolt" />}
            <span className="hidden 2xl:inline">Симулировать</span>
            <span>ИИ-крон</span>
          </Button>
        )}
        <button
          type="button"
          onClick={toggleInspector}
          aria-pressed={inspectorOpen}
          aria-label="Инспектор MCP & RBAC"
          title="Под капотом: MCP · RBAC · Clickstream"
          className={cn(
            'inline-flex h-10 shrink-0 items-center gap-2 whitespace-nowrap rounded-card border px-3 text-meta font-semibold transition-all',
            FOCUS,
            inspectorOpen ? 'border-ink-900 bg-ink-900 text-white shadow-card' : 'border-line bg-white text-ink-900 hover:border-ink-400/60 hover:shadow-card',
          )}
        >
          <StatusDot tone={inspectorOpen ? 'ok' : 'neutral'} pulse={inspectorOpen} />
          Инспектор
          {events !== undefined && (
            <span className={cn('rounded-tag px-1 font-mono', inspectorOpen ? 'bg-white/15 text-white' : 'bg-ink-900/5 text-ink-500')}>{events}</span>
          )}
        </button>
        <Button
          variant="ghost"
          size="icon"
          className="shrink-0"
          onClick={() => reset.mutate()}
          loading={reset.isPending}
          aria-label="Сбросить демо"
          title="Вернуть демо-данные в исходное состояние"
        >
          {!reset.isPending && <Icon name="reset" />}
        </Button>
      </div>
    </header>
  );
}
