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

export interface DemoInternProfile {
  id: string;
  name: string;
  roleTitle: string;
  team: string;
  stage: string;
  stack: string;
  mentorName: string;
  token: string;
}

export const DEMO_INTERNS: DemoInternProfile[] = [
  {
    id: 'u-intern-01',
    name: 'Алексей Смирнов',
    roleTitle: 'Backend Go Intern',
    team: 'Автотека',
    stage: 'Неделя 1',
    stack: 'Go',
    mentorName: 'Мария Кузнецова',
    token: 'demo-intern-token',
  },
  {
    id: 'u-intern-02',
    name: 'Мария Козлова',
    roleTitle: 'Python/ML Intern',
    team: 'Авито Товары (Модерация)',
    stage: 'Неделя 3',
    stack: 'Python/ML',
    mentorName: 'Елена Соколова',
    token: 'demo-intern-02-token',
  },
];

let currentRole: DemoRole = 'intern';
let currentInternId: string = 'u-intern-01';

/** Устанавливает роль, от имени которой клиент выполняет запросы. */
export function setApiRole(role: DemoRole): void {
  currentRole = role;
}

export function getApiRole(): DemoRole {
  return currentRole;
}

/** Устанавливает ID активного стажёра для контекста API. */
export function setApiInternId(id: string): void {
  currentInternId = id;
}

export function getApiInternId(): string {
  return currentInternId;
}

/** Общий экземпляр API-клиента приложения. */
export const api = createApiClient({
  baseUrl: import.meta.env.VITE_API_URL ?? '',
  getToken: () => {
    if (currentRole === 'intern') {
      const intern = DEMO_INTERNS.find((i) => i.id === currentInternId);
      return intern?.token ?? DEMO_USERS.intern.token;
    }
    return DEMO_USERS[currentRole].token;
  },
  getInternId: () => currentInternId,
});
