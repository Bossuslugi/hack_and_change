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

export interface UiState {
  role: DemoRole;
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
      inspectorOpen: false,
      inspectorTab: 'mcp',
      toast: null,
      ...sessionDefaults,

      setRole: (role) => {
        setApiRole(role);
        set({ role, ...sessionDefaults });
      },
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
      resetSession: () => set({ ...sessionDefaults }),
    }),
    {
      name: 'ai-buddy-ui',
      partialize: (state) => ({ role: state.role }),
      // После восстановления роли из localStorage синхронизируем её с API-клиентом.
      onRehydrateStorage: () => (state) => {
        if (state) setApiRole(state.role);
      },
    },
  ),
);
