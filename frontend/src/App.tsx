import { useEffect } from 'react';
import { AdaptationMap } from './components/AdaptationMap';
import { ChatPanel } from './components/chat/ChatPanel';
import { Header } from './components/Header';
import { Inspector } from './components/inspector/Inspector';
import { LeadPanel, MentorPanel } from './components/RolePanels';
import { Icon } from './components/ui/icons';
import { Button, FOCUS, Skeleton, cn } from './components/ui/primitives';
import { errorText, useSendMessage, useWorkspace } from './hooks/useBuddy';
import { useUi } from './store/ui';

function Toast() {
  const toast = useUi((s) => s.toast);
  const hide = useUi((s) => s.hideToast);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(hide, 6000);
    return () => clearTimeout(timer);
  }, [toast, hide]);
  if (!toast) return null;
  const marker = toast.kind === 'error' ? 'bg-danger-500' : toast.kind === 'success' ? 'bg-ok-500' : 'bg-ai-500';
  return (
    <div role="status" className="fixed bottom-4 left-1/2 z-40 flex max-w-[92vw] -translate-x-1/2 items-center gap-2 rounded-card bg-ink-900 py-2 pl-4 pr-2 text-body text-white shadow-float">
      <span className={cn('h-2 w-2 shrink-0 rounded-full', marker)} aria-hidden="true" />
      <span>{toast.text}</span>
      <button
        type="button"
        onClick={hide}
        className={cn('grid h-8 w-8 shrink-0 place-items-center rounded-tag text-white/60 transition-colors hover:text-white', FOCUS)}
        aria-label="Скрыть уведомление"
      >
        <Icon name="close" />
      </button>
    </div>
  );
}

/** Каркас трёх зон на время загрузки: те же размеры, что у готового экрана, — без скачков вёрстки. */
function WorkspaceSkeleton() {
  return (
    <>
      <div className="flex w-[42%] shrink-0 flex-col gap-2" aria-hidden="true">
        <Skeleton className="h-32 w-full rounded-card" />
        <div className="grid grid-cols-2 gap-2">
          <Skeleton className="h-32 rounded-card" />
          <Skeleton className="h-32 rounded-card" />
        </div>
        <Skeleton className="min-h-0 w-full flex-1 rounded-card" />
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-4 rounded-panel border border-line bg-white p-4 shadow-card" aria-hidden="true">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="mt-auto h-24 w-full rounded-card" />
      </div>
      <p role="status" className="sr-only">
        Загружаю рабочее пространство…
      </p>
    </>
  );
}

export default function App() {
  const role = useUi((s) => s.role);
  const workspaceQuery = useWorkspace();
  const send = useSendMessage();
  const workspace = workspaceQuery.data;
  // Данные другой роли не показываем даже на мгновение при переключении.
  const ready = workspace && (role === 'intern') === (workspace.viewer.role === 'intern_backend');
  const failed = workspaceQuery.isError && !workspace;

  const ask = (text: string) => {
    if (!send.isPending) send.mutate(text);
  };

  return (
    <div className="app-canvas flex h-screen flex-col overflow-hidden font-sans text-ink-900">
      <Header workspace={ready ? workspace : undefined} />
      <div className="flex min-h-0 flex-1 gap-4 p-4">
        <main className="flex min-h-0 min-w-0 flex-1 gap-4">
          {failed && (
            <div role="alert" className="m-auto w-full max-w-md rounded-panel border border-line bg-white p-6 text-center shadow-card">
              <p className="text-body font-semibold text-ink-900">Не удалось загрузить рабочее пространство</p>
              <p className="mt-1 text-body text-ink-500">{errorText(workspaceQuery.error)}</p>
              <Button variant="primary" className="mt-4" onClick={() => void workspaceQuery.refetch()} loading={workspaceQuery.isFetching}>
                Повторить
              </Button>
            </div>
          )}
          {!ready && !failed && <WorkspaceSkeleton />}
          {ready && (
            <>
              <AdaptationMap workspace={workspace} onAsk={workspace.permissions.chat ? ask : undefined} />
              <div className="flex min-h-0 min-w-0 flex-1 flex-col">
                {role === 'intern' && <ChatPanel workspace={workspace} onSend={ask} busy={send.isPending} />}
                {role === 'mentor' && <MentorPanel workspace={workspace} />}
                {role === 'lead' && <LeadPanel workspace={workspace} />}
              </div>
            </>
          )}
        </main>
        <Inspector />
      </div>
      <Toast />
    </div>
  );
}
