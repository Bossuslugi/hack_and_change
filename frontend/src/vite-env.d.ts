/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Базовый URL API. Пусто — тот же origin (nginx / dev-proxy). */
  readonly VITE_API_URL?: string;
}
