import { useQueryClient } from '@tanstack/react-query';
import { api, DEMO_INTERNS, DEMO_ROLES, DEMO_USERS, type DemoRole } from '../api/auth';
import type { Workspace } from '../api/schemas';
import { useCron, useReset } from '../hooks/useBuddy';
import { internContext } from '../lib/format';
import { useUi } from '../store/ui';
import { Icon, type IconName } from './ui/icons';
import { Badge, Button, FOCUS, cn } from './ui/primitives';

const ROLE_ICON: Record<DemoRole, IconName> = { intern: 'user', mentor: 'mentor', lead: 'chart' };

function RoleSwitcher({ stack }: { stack?: string }) {
  const role = useUi((s) => s.role);
  const setRole = useUi((s) => s.setRole);

  const choose = (next: DemoRole) => {
    if (next === role) return;
    setRole(next);
    void api.trackEvent('ui.role_switched', { role: next });
  };

  const roleHint = (r: DemoRole) => {
    if (r === 'intern') return stack?.includes('ML') ? 'ML Intern' : 'Go Intern';
    if (r === 'mentor') return 'бриф к 1:1';
    return 'метрики';
  };

  return (
    <div role="radiogroup" aria-label="Роль пользователя" className="inline-flex h-10 items-center gap-1 rounded-card bg-ink-900/5 p-1">
      {DEMO_ROLES.map((r) => {
        const user = DEMO_USERS[r];
        const active = role === r;
        return (
          <button
            key={r}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={user.label}
            title={`${user.label}: ${user.name}`}
            onClick={() => choose(r)}
            className={cn(
              'inline-flex h-8 items-center gap-2 rounded-tag px-2 text-body font-medium transition-colors',
              FOCUS,
              active ? 'bg-white text-ink-900 shadow-card' : 'text-ink-500 hover:text-ink-900',
            )}
          >
            <Icon name={ROLE_ICON[r]} className={active ? 'text-accent-500' : undefined} />
            <span>{user.label}</span>
            <span className="hidden font-mono text-meta font-normal text-ink-400 2xl:inline">
              {user.name.split(' ')[0]} · {roleHint(r)}
            </span>
          </button>
        );
      })}
    </div>
  );
}

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
    <div
      role="radiogroup"
      aria-label="Профиль стажёра"
      className="inline-flex h-8 items-center gap-1 rounded-card bg-ink-900/5 p-0.5"
    >
      {DEMO_INTERNS.map((intern) => {
        const active = activeInternId === intern.id;
        return (
          <button
            key={intern.id}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={`${intern.name} (${intern.team})`}
            title={`${intern.name} — ${intern.roleTitle} • ${intern.team} • ${intern.stage}`}
            onClick={() => choose(intern.id)}
            className={cn(
              'inline-flex h-7 items-center gap-1.5 rounded-tag px-2 text-meta font-medium transition-colors',
              FOCUS,
              active ? 'bg-white text-ink-900 shadow-card' : 'text-ink-500 hover:text-ink-900',
            )}
          >
            <span
              className={cn(
                'h-2 w-2 rounded-full',
                intern.stack === 'Go' ? 'bg-[#0095E0]' : 'bg-[#9B51E0]',
              )}
              aria-hidden="true"
            />
            <span>{intern.name.split(' ')[0]}</span>
            <span className="hidden xl:inline font-mono text-meta font-normal text-ink-400">
              · {intern.stack === 'Go' ? 'Go' : 'ML'}
            </span>
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

  const toggleInspector = () => {
    const ui = useUi.getState();
    if (inspectorOpen) return ui.closeInspector();
    ui.openInspector();
    void api.trackEvent('ui.inspector_opened', {});
  };

  return (
    <header className="glass relative z-20 grid h-14 shrink-0 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-4 border-x-0 border-t-0 px-4 shadow-card">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex shrink-0 items-center gap-2">
          <span className="grid h-8 w-8 place-items-center rounded-tag bg-accent-500 text-white" aria-hidden="true">
            <svg viewBox="0 0 32 32" className="h-5 w-5">
              <path d="M10 22V10l7 6-7 6Z" fill="currentColor" />
              <path d="M19 10l5 6-5 6" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </span>
          <p className="text-body font-semibold text-ink-900">
            Avito Start <span className="font-normal text-ink-400">AI-Buddy</span>
          </p>
        </div>
        <InternSwitcher />
        {workspace && (
          <Badge className="hidden min-w-0 xl:inline-flex" title={`${workspace.intern.name} · ${workspace.intern.program}`}>
            <span className="truncate" aria-label="Контекст стажёра">
              {internContext(workspace.intern)}
            </span>
          </Badge>
        )}
      </div>

      <RoleSwitcher stack={workspace?.intern.stack} />

      <div className="flex min-w-0 items-center justify-end gap-2">
        {workspace && (
          <Badge
            mono
            tone={workspace.engine.mode === 'llm' ? 'ok' : 'neutral'}
            className="hidden lg:inline-flex"
            title={`Хранилище: ${workspace.storage.store} · кэш: ${workspace.storage.cache}`}
          >
            {workspace.engine.mode === 'llm' ? `LLM · ${workspace.engine.model}` : 'Smart Engine'}
          </Badge>
        )}
        {workspace?.permissions.cron && (
          <Button
            variant="ai"
            size="sm"
            onClick={() => cron.mutate()}
            loading={cron.isPending}
            aria-label="Симулировать ИИ-крон"
            title="Проверить застрявшие задачи стажёра и собрать наставнику бриф к 1:1"
          >
            {!cron.isPending && <Icon name="bolt" />}
            <span className="hidden 2xl:inline">Симулировать</span> ИИ-крон
          </Button>
        )}
        <button
          type="button"
          onClick={toggleInspector}
          aria-pressed={inspectorOpen}
          aria-label="Инспектор MCP & RBAC"
          title="Под капотом: MCP · RBAC · Clickstream"
          className={cn(
            'inline-flex h-8 shrink-0 items-center gap-2 rounded-tag border border-line bg-white px-2 text-meta font-medium text-ink-900 transition-colors hover:bg-canvas',
            FOCUS,
          )}
        >
          <Icon name="sliders" className="text-ink-500" />
          Инспектор
          <span className={cn('relative h-4 w-7 rounded-full transition-colors', inspectorOpen ? 'bg-accent-500' : 'bg-ink-400/50')} aria-hidden="true">
            <span className={cn('absolute top-0.5 h-3 w-3 rounded-full bg-white transition-[left]', inspectorOpen ? 'left-3.5' : 'left-0.5')} />
          </span>
        </button>
        <Button
          variant="ghost"
          size="sm"
          className="w-8 px-0"
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
