// Клиентское состояние интерфейса (Zustand): выбранная роль, открытые панели и
// оптимистичный UI чата. Серверное состояние живёт в TanStack Query (src/hooks/useBuddy.ts).
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { setApiRole, type DemoRole } from '../api/auth';
import type { CronResult, Stage } from '../api/schemas';

export type InspectorTab = 'mcp' | 'rbac' | 'clickstream' | 'registry';

export interface ToastState {
  kind: 'info' | 'success' | 'error';
  text: string;
}

export interface StreamState {
  stages: Stage[];
  text: string;
}

export interface ChatErrorState {
  message: string;
  /** Текст сообщения для повторной отправки. */
  retryText: string;
}

export type Theme = 'light' | 'dark';

/** Применяет тему к документу (класс .dark на <html>). */
export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark');
}

function systemTheme(): Theme {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export interface UiState {
  role: DemoRole;
  theme: Theme;
  /** Напоминания, для которых уже показано всплывающее уведомление. */
  seenReminders: string[];
  inspectorOpen: boolean;
  inspectorTab: InspectorTab;
  /** Сообщение пользователя, показанное до ответа сервера (оптимистичный UI). */
  pendingText: string | null;
  /** Поток ответа агента: этапы и уже полученный текст. */
  stream: StreamState | null;
  chatError: ChatErrorState | null;
  toast: ToastState | null;
  cronResult: CronResult | null;

  setRole: (role: DemoRole) => void;
  toggleTheme: () => void;
  markRemindersSeen: (ids: string[]) => void;
  openInspector: (tab?: InspectorTab) => void;
  closeInspector: () => void;
  setInspectorTab: (tab: InspectorTab) => void;
  beginSend: (text: string) => void;
  pushStage: (stage: Stage) => void;
  pushDelta: (text: string) => void;
  endSend: () => void;
  failSend: (message: string, retryText: string) => void;
  clearChatError: () => void;
  showToast: (toast: ToastState) => void;
  hideToast: () => void;
  setCronResult: (result: CronResult | null) => void;
  resetSession: () => void;
}

const sessionDefaults = {
  pendingText: null,
  stream: null,
  chatError: null,
  cronResult: null,
} satisfies Partial<UiState>;

export const useUi = create<UiState>()(
  persist(
    (set) => ({
      role: 'intern',
      theme: systemTheme(),
      seenReminders: [],
      inspectorOpen: false,
      inspectorTab: 'mcp',
      toast: null,
      ...sessionDefaults,

      setRole: (role) => {
        setApiRole(role);
        set({ role, ...sessionDefaults });
      },
      toggleTheme: () =>
        set((s) => {
          const theme: Theme = s.theme === 'dark' ? 'light' : 'dark';
          applyTheme(theme);
          return { theme };
        }),
      markRemindersSeen: (ids) => set((s) => ({ seenReminders: [...new Set([...s.seenReminders, ...ids])] })),
      openInspector: (tab) => set((s) => ({ inspectorOpen: true, inspectorTab: tab ?? s.inspectorTab })),
      closeInspector: () => set({ inspectorOpen: false }),
      setInspectorTab: (inspectorTab) => set({ inspectorTab }),

      beginSend: (text) => set({ pendingText: text, stream: { stages: [], text: '' }, chatError: null }),
      pushStage: (stage) =>
        set((s) => (s.stream ? { stream: { ...s.stream, stages: [...s.stream.stages, stage] } } : {})),
      pushDelta: (text) => set((s) => (s.stream ? { stream: { ...s.stream, text: s.stream.text + text } } : {})),
      endSend: () => set({ pendingText: null, stream: null }),
      failSend: (message, retryText) => set({ pendingText: null, stream: null, chatError: { message, retryText } }),
      clearChatError: () => set({ chatError: null }),

      showToast: (toast) => set({ toast }),
      hideToast: () => set({ toast: null }),
      setCronResult: (cronResult) => set({ cronResult }),
      resetSession: () => set({ ...sessionDefaults, seenReminders: [] }),
    }),
    {
      name: 'ai-buddy-ui',
      partialize: (state) => ({ role: state.role, theme: state.theme }),
      // После восстановления роли из localStorage синхронизируем её с API-клиентом.
      onRehydrateStorage: () => (state) => {
        if (state) {
          setApiRole(state.role);
          applyTheme(state.theme);
        }
      },
    },
  ),
);
