// Имитация корпоративной авторизации: переключатель ролей выбирает тестового пользователя.
// Токены — публичные тестовые учётные данные из README, не секреты. Роль определяет сервер
// по токену; клиент не может «назначить» себе роль.
import { createApiClient } from './client';

export type DemoRole = 'intern' | 'mentor' | 'lead';

export interface DemoUser {
  role: DemoRole;
  label: string;
  name: string;
  token: string;
}

export const DEMO_USERS: Record<DemoRole, DemoUser> = {
  intern: { role: 'intern', label: 'Стажёр', name: 'Алексей Смирнов', token: 'demo-intern-token' },
  mentor: { role: 'mentor', label: 'Наставник', name: 'Мария Кузнецова', token: 'demo-mentor-token' },
  lead: { role: 'lead', label: 'Руководитель', name: 'Дмитрий Орлов', token: 'demo-lead-token' },
};

export const DEMO_ROLES: DemoRole[] = ['intern', 'mentor', 'lead'];

let currentRole: DemoRole = 'intern';

/** Устанавливает роль, от имени которой клиент выполняет запросы. */
export function setApiRole(role: DemoRole): void {
  currentRole = role;
}

export function getApiRole(): DemoRole {
  return currentRole;
}

/** Общий экземпляр API-клиента приложения. */
export const api = createApiClient({
  baseUrl: import.meta.env.VITE_API_URL ?? '',
  getToken: () => DEMO_USERS[currentRole].token,
});
