import { useEffect } from 'react';
import { ChatPanel } from './components/chat/ChatPanel';
import { GoffyAvatar } from './components/GoffyAvatar';
import { Header } from './components/Header';
import { Inspector } from './components/inspector/Inspector';
import { useReminderToasts } from './components/Planner';
import { LeadPanel, MentorPanel } from './components/RolePanels';
import { Sidebar } from './components/Sidebar';
import { Button, cn } from './components/ui/primitives';
import { errorText, useSendMessage, useWorkspace } from './hooks/useBuddy';
import { useUi } from './store/ui';

function Toast() {
  const toast = useUi((s) => s.toast);
  const hide = useUi((s) => s.hideToast);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(hide, 7000);
    return () => clearTimeout(timer);
  }, [toast, hide]);
  if (!toast) return null;
  return (
    <div
      role="status"
      data-testid="toast"
      className={cn(
        'fixed bottom-4 left-1/2 z-50 flex max-w-[92vw] -translate-x-1/2 animate-slide-in items-center gap-3 rounded-2xl px-4 py-2.5 text-sm text-white shadow-drawer ring-1 ring-white/10 backdrop-blur-xl',
        toast.kind === 'error' ? 'bg-red-700/95' : toast.kind === 'success' ? 'bg-emerald-700/95' : 'bg-slate-800/95',
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
  useReminderToasts(ready && role === 'intern' ? workspace.reminders : undefined);

  const ask = (text: string) => {
    if (!send.isPending) send.mutate(text);
  };

  return (
    <div className="app-backdrop relative flex min-h-screen w-full flex-col font-sans text-ink-900 lg:h-screen lg:overflow-hidden">
      <div aria-hidden="true">
        <span className="orb -left-40 -top-40 h-[32rem] w-[32rem] bg-[#00AAFF]" />
        <span className="orb -right-40 top-1/4 h-[28rem] w-[28rem] bg-[#8B5CF6]" />
        <span className="orb -bottom-48 left-1/3 h-[30rem] w-[30rem] bg-[#00C257]" />
      </div>
      <Header workspace={ready ? workspace : undefined} />
      <main className="relative mx-auto flex min-h-0 w-full max-w-[1920px] flex-1 flex-col p-4 lg:overflow-hidden">
        {workspaceQuery.isError && !workspace && (
          <div role="alert" className="bento mx-auto mt-16 w-full max-w-md p-6 text-center">
            <p className="text-sm font-semibold text-ink-900">Не удалось загрузить рабочее пространство</p>
            <p className="mt-1 text-sm text-ink-500">{errorText(workspaceQuery.error)}</p>
            <Button variant="primary" className="mt-4" onClick={() => void workspaceQuery.refetch()} loading={workspaceQuery.isFetching}>
              Повторить
            </Button>
          </div>
        )}
        {!ready && !workspaceQuery.isError && (
          <div className="mt-24 flex flex-col items-center justify-center gap-3 text-sm text-ink-500">
            <GoffyAvatar size="lg" pulse />
            <p>Гоффи загружает рабочее пространство…</p>
          </div>
        )}
        {ready && (
          <div className="grid min-h-0 flex-1 grid-cols-12 gap-4">
            <div className="order-2 col-span-12 flex min-h-0 flex-col lg:order-1 lg:col-span-7">
              <Sidebar workspace={workspace} onAsk={workspace.permissions.chat ? ask : undefined} />
            </div>
            <div className="order-1 col-span-12 flex min-h-0 min-w-0 flex-col lg:order-2 lg:col-span-5">
              {role === 'intern' && <ChatPanel workspace={workspace} onSend={ask} busy={send.isPending} />}
              {role === 'mentor' && <MentorPanel workspace={workspace} />}
              {role === 'lead' && <LeadPanel workspace={workspace} />}
            </div>
          </div>
        )}
      </main>
      <Inspector workspace={ready ? workspace : undefined} />
      <Toast />
    </div>
  );
}
