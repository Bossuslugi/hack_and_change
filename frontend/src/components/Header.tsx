import { useQueryClient } from '@tanstack/react-query';
import { api, DEMO_INTERNS, DEMO_ROLES, DEMO_USERS, type DemoRole } from '../api/auth';
import { useReset } from '../hooks/useBuddy';
import { useUi } from '../store/ui';
import { GoffyAvatar } from './GoffyAvatar';
import { Button, cn } from './ui/primitives';

function RoleSwitcher() {
  const role = useUi((s) => s.role);
  const setRole = useUi((s) => s.setRole);

  const choose = (next: DemoRole) => {
    if (next === role) return;
    setRole(next);
    void api.trackEvent('ui.role_switched', { role: next });
  };

  return (
    <div role="radiogroup" aria-label="Роль пользователя" className="inline-flex shrink-0 whitespace-nowrap rounded-2xl bg-paper-100 p-1">
      {DEMO_ROLES.map((r) => (
        <button
          key={r}
          type="button"
          role="radio"
          aria-checked={role === r}
          title={`${DEMO_USERS[r].label}: ${DEMO_USERS[r].name}`}
          onClick={() => choose(r)}
          className={cn(
            'rounded-xl px-3 py-1.5 text-sm font-medium transition-all duration-200',
            'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-500',
            role === r ? 'bg-paper-0 text-ink-900 shadow-card' : 'text-ink-500 hover:text-ink-900',
          )}
        >
          {DEMO_USERS[r].label}
        </button>
      ))}
    </div>
  );
}

/** Профиль стажёра: переключает демо-данные между Go- и ML-стажёром (несколько команд). */
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
    <div role="radiogroup" aria-label="Профиль стажёра" className="inline-flex shrink-0 whitespace-nowrap rounded-2xl bg-paper-100 p-1">
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
              'inline-flex items-center gap-1.5 rounded-xl px-2.5 py-1.5 text-sm font-medium transition-all duration-200',
              'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-500',
              active ? 'bg-paper-0 text-ink-900 shadow-card' : 'text-ink-500 hover:text-ink-900',
            )}
          >
            <span className={cn('h-2 w-2 rounded-full', intern.stack === 'Go' ? 'bg-brand-500' : 'bg-accent-500')} aria-hidden="true" />
            {intern.name.split(' ')[0]}
            <span className="font-mono text-[11px] font-normal text-ink-400">{intern.stack === 'Go' ? 'Go' : 'ML'}</span>
          </button>
        );
      })}
    </div>
  );
}

/** Переключатель светлой и тёмной темы; выбор сохраняется в localStorage. */
function ThemeToggle() {
  const theme = useUi((s) => s.theme);
  const toggle = useUi((s) => s.toggleTheme);
  const dark = theme === 'dark';
  return (
    <button
      type="button"
      role="switch"
      aria-checked={dark}
      aria-label="Тёмная тема"
      title={dark ? 'Включить светлую тему' : 'Включить тёмную тему'}
      onClick={() => {
        toggle();
        void api.trackEvent('ui.theme_switched', { theme: dark ? 'light' : 'dark' });
      }}
      className="grid h-9 w-9 place-items-center rounded-xl bg-paper-100 text-ink-700 transition-all duration-300 hover:text-brand-500 hover:shadow-glow focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500"
    >
      <svg viewBox="0 0 24 24" className={cn('h-[18px] w-[18px] transition-transform duration-500', dark ? 'rotate-0' : 'rotate-90')} aria-hidden="true">
        {dark ? (
          <path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z" fill="currentColor" />
        ) : (
          <g fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <circle cx="12" cy="12" r="4" fill="currentColor" stroke="none" />
            <path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
          </g>
        )}
      </svg>
    </button>
  );
}

export function Header() {
  const openInspector = useUi((s) => s.openInspector);
  const inspectorOpen = useUi((s) => s.inspectorOpen);
  const reset = useReset();

  const toggleInspector = () => {
    if (inspectorOpen) return useUi.getState().closeInspector();
    openInspector();
    void api.trackEvent('ui.inspector_opened', {});
  };

  return (
    <header className="sticky top-0 z-20 shrink-0 border-b border-paper-200 bg-paper-0/70 backdrop-blur-xl transition-colors duration-300">
      <div className="mx-auto flex max-w-[1920px] flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2.5 lg:flex-nowrap">
        <div className="flex shrink-0 items-center gap-2.5">
          <GoffyAvatar size="sm" className="h-9 w-9" />
          <p className="whitespace-nowrap text-sm font-semibold text-ink-900">
            Гоффи <span className="font-normal text-ink-400">· Avito Start</span>
          </p>
        </div>

        <div className="ml-auto flex flex-wrap items-center gap-2 lg:flex-nowrap">
          <InternSwitcher />
          <RoleSwitcher />
          <ThemeToggle />
          <Button className="whitespace-nowrap" onClick={toggleInspector} aria-pressed={inspectorOpen} title="MCP · JSON-RPC · RBAC · Clickstream">
            <svg viewBox="0 0 20 20" className="h-4 w-4" aria-hidden="true">
              <path d="M3 5h14M3 10h9M3 15h12" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" fill="none" />
            </svg>
            Инспектор жюри
          </Button>
          <Button variant="ghost" className="whitespace-nowrap" onClick={() => reset.mutate()} loading={reset.isPending} title="Вернуть демо-данные в исходное состояние">
            Сбросить демо
          </Button>
        </div>
      </div>
      {/* Дисклеймер остаётся для скринридеров; видимые плашки синтетики стоят у каждой инструкции в чате. */}
      <p className="sr-only">
        Синтетические данные: сотрудники, инструкции и корпоративные сервисы — имитации. Инструкции не являются регламентами Авито.
      </p>
    </header>
  );
}
