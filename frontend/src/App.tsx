import { useEffect } from 'react';
import { ChatPanel } from './components/chat/ChatPanel';
import { Header } from './components/Header';
import { Inspector } from './components/inspector/Inspector';
import { LeadPanel, MentorPanel } from './components/RolePanels';
import { Sidebar } from './components/Sidebar';
import { Button, Spinner, cn } from './components/ui/primitives';
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
  return (
    <div
      role="status"
      className={cn(
        'fixed bottom-4 left-1/2 z-40 flex max-w-[92vw] -translate-x-1/2 items-center gap-3 rounded-xl px-4 py-2.5 text-sm text-white shadow-drawer',
        toast.kind === 'error' ? 'bg-danger-700' : toast.kind === 'success' ? 'bg-ok-700' : 'bg-ink-900',
      )}
    >
      <span>{toast.text}</span>
      <button type="button" onClick={hide} className="text-white/70 hover:text-white" aria-label="Скрыть уведомление">
        ✕
      </button>
    </div>
  );
}

export default function App() {
  const role = useUi((s) => s.role);
  const workspaceQuery = useWorkspace();
  const send = useSendMessage();
  const workspace = workspaceQuery.data;
  // Данные другой роли не показываем даже на мгновение при переключении.
  const ready = workspace && (role === 'intern') === (workspace.viewer.role === 'intern_backend');

  const ask = (text: string) => {
    if (!send.isPending) send.mutate(text);
  };

  return (
    <div className="min-h-screen bg-paper-50 font-sans text-ink-900">
      <Header workspace={ready ? workspace : undefined} />
      <main className="mx-auto max-w-[1440px] px-4 py-4">
        {workspaceQuery.isError && !workspace && (
          <div role="alert" className="mx-auto mt-16 max-w-md rounded-2xl bg-white p-6 text-center shadow-card ring-1 ring-paper-200">
            <p className="text-sm font-semibold text-ink-900">Не удалось загрузить рабочее пространство</p>
            <p className="mt-1 text-sm text-ink-500">{errorText(workspaceQuery.error)}</p>
            <Button variant="primary" className="mt-4" onClick={() => void workspaceQuery.refetch()} loading={workspaceQuery.isFetching}>
              Повторить
            </Button>
          </div>
        )}
        {!ready && !workspaceQuery.isError && (
          <div className="mt-24 flex items-center justify-center gap-2 text-sm text-ink-500" role="status">
            <Spinner className="text-brand-500" /> Загружаю рабочее пространство…
          </div>
        )}
        {ready && (
          <div className="grid gap-4 lg:grid-cols-[340px_minmax(0,1fr)]">
            <div className="order-2 lg:order-1">
              <Sidebar workspace={workspace} onAsk={workspace.permissions.chat ? ask : undefined} />
            </div>
            <div className="order-1 min-w-0 lg:order-2">
              {role === 'intern' && <ChatPanel workspace={workspace} onSend={ask} busy={send.isPending} />}
              {role === 'mentor' && <MentorPanel workspace={workspace} />}
              {role === 'lead' && <LeadPanel workspace={workspace} />}
            </div>
          </div>
        )}
      </main>
      <Inspector />
      <Toast />
    </div>
  );
}
