import { api, DEMO_ROLES, DEMO_USERS, type DemoRole } from '../api/auth';
import type { Workspace } from '../api/schemas';
import { useReset } from '../hooks/useBuddy';
import { useUi } from '../store/ui';
import { Badge, Button, cn } from './ui/primitives';

function RoleSwitcher() {
  const role = useUi((s) => s.role);
  const setRole = useUi((s) => s.setRole);

  const choose = (next: DemoRole) => {
    if (next === role) return;
    setRole(next);
    void api.trackEvent('ui.role_switched', { role: next });
  };

  return (
    <div role="radiogroup" aria-label="Роль пользователя" className="inline-flex rounded-xl bg-paper-100 p-1">
      {DEMO_ROLES.map((r) => (
        <button
          key={r}
          type="button"
          role="radio"
          aria-checked={role === r}
          title={`${DEMO_USERS[r].label}: ${DEMO_USERS[r].name}`}
          onClick={() => choose(r)}
          className={cn(
            'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
            'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-500',
            role === r ? 'bg-white text-ink-900 shadow-card' : 'text-ink-500 hover:text-ink-900',
          )}
        >
          {DEMO_USERS[r].label}
        </button>
      ))}
    </div>
  );
}

export function Header({ workspace }: { workspace?: Workspace }) {
  const openInspector = useUi((s) => s.openInspector);
  const inspectorOpen = useUi((s) => s.inspectorOpen);
  const reset = useReset();

  const toggleInspector = () => {
    if (inspectorOpen) return useUi.getState().closeInspector();
    openInspector();
    void api.trackEvent('ui.inspector_opened', {});
  };

  return (
    <header className="sticky top-0 z-20 border-b border-paper-200 bg-white/95 backdrop-blur">
      <div className="mx-auto flex max-w-[1440px] flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2.5">
        <div className="flex items-center gap-2.5">
          <div className="grid h-9 w-9 place-items-center rounded-xl bg-brand-500 text-white" aria-hidden="true">
            <svg viewBox="0 0 24 24" className="h-5 w-5">
              <path d="M5 17V8a3 3 0 0 1 3-3h8a3 3 0 0 1 3 3v5a3 3 0 0 1-3 3h-5l-6 4z" fill="currentColor" />
            </svg>
          </div>
          <div className="leading-tight">
            <p className="text-sm font-semibold text-ink-900">AI-Buddy</p>
            <p className="text-[11px] text-ink-500">ИИ-наставник для стажёров · прототип</p>
          </div>
        </div>

        {workspace && (
          <div className="hidden items-center gap-1.5 lg:flex" aria-label="Контекст стажёра">
            <Badge>{workspace.intern.name}</Badge>
            <Badge tone="brand">Команда {workspace.intern.team}</Badge>
            <Badge tone="brand">{workspace.intern.stack}</Badge>
            <Badge tone="brand">{workspace.intern.stage}</Badge>
          </div>
        )}

        <div className="ml-auto flex flex-wrap items-center gap-2">
          {workspace && (
            <Badge
              tone={workspace.engine.mode === 'llm' ? 'ok' : 'neutral'}
              title={`Хранилище: ${workspace.storage.store} · кэш: ${workspace.storage.cache}`}
            >
              {workspace.engine.mode === 'llm' ? `LLM · ${workspace.engine.model}` : 'Smart Engine'}
            </Badge>
          )}
          <RoleSwitcher />
          <Button onClick={toggleInspector} aria-pressed={inspectorOpen} title="MCP · RBAC · Clickstream">
            <svg viewBox="0 0 20 20" className="h-4 w-4" aria-hidden="true">
              <path d="M3 5h14M3 10h9M3 15h12" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" fill="none" />
            </svg>
            Инспектор
          </Button>
          <Button variant="ghost" onClick={() => reset.mutate()} loading={reset.isPending} title="Вернуть демо-данные в исходное состояние">
            Сбросить демо
          </Button>
        </div>
      </div>
      <p className="border-t border-warn-500/20 bg-warn-50 px-4 py-1 text-center text-[11px] text-warn-700">
        Синтетические данные: сотрудники, инструкции и корпоративные сервисы — имитации. Инструкции не являются регламентами Авито.
      </p>
    </header>
  );
}
