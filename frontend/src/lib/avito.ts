// Ориентиры из Q&A с организаторами кейса Авито. Это справочные цифры процесса онбординга,
// а не данные прототипа: в интерфейсе они показаны отдельно от метрик, посчитанных бэкендом.

export interface Benchmark {
  id: string;
  label: string;
  value: string;
  note: string;
  tone: 'brand' | 'ok' | 'warn' | 'accent';
}

export const AVITO_BENCHMARKS: Benchmark[] = [
  { id: 'adaptation', label: 'Длительность адаптации', value: '~2 мес', note: 'Средний срок выхода стажёра на рабочий ритм команды.', tone: 'brand' },
  {
    id: 'ttft',
    label: 'Time-to-First-Task',
    value: '~2 нед',
    note: 'До первой самостоятельной задачи. Гипотеза с Гоффи: 7–9 дней.',
    tone: 'accent',
  },
  { id: 'mentor_csat', label: 'Удовлетворённость наставником', value: '90%', note: 'Сильная сторона процесса, её нужно сохранить.', tone: 'ok' },
  { id: 'onboarding_csat', label: 'Удовлетворённость онбордингом', value: '9/10', note: 'Общая оценка программы стажёрами.', tone: 'ok' },
  {
    id: 'checklist_csat',
    label: 'Удовлетворённость чек-листом',
    value: '68%',
    note: 'Главная точка роста: статичная таблица. Гоффи делает её навигатором, прогноз — 92%+.',
    tone: 'warn',
  },
];

/** Нагрузка наставника по фазам (часы в неделю) и суммарно за программу. */
export const MENTOR_LOAD = {
  phases: [
    { id: 'adaptation', label: 'Первые 2–3 месяца', hours: '~5 ч/нед', share: 100, note: 'Доступы, задачи, погружение, 1:1' },
    { id: 'steady', label: 'После адаптации', hours: '~2–3 ч/нед', share: 50, note: '1:1, ревью, ИПР и матрица компетенций' },
  ],
  totals: [
    { label: 'за 6 месяцев', value: '40–48 ч' },
    { label: 'за 9 месяцев', value: '55–65 ч' },
  ],
} as const;

/** Корпоративные сервисы из Q&A и то, зачем они нужны стажёру. */
export const AVITO_SERVICES: Array<{ name: string; purpose: string }> = [
  { name: 'Stash', purpose: 'Git-репозитории' },
  { name: 'Jira', purpose: 'бэклоги и тикеты' },
  { name: 'Confluence', purpose: 'спеки и гайды' },
  { name: 'Redash', purpose: 'продуктовые метрики' },
  { name: 'Grafana', purpose: 'дашборды сервисов' },
];

/** Какие сервисы затрагивает шаг чек-листа. */
export const STEP_SERVICES: Record<string, string[]> = {
  access_repo: ['Stash', 'Redash'],
  local_env: ['Stash'],
  team_processes: ['Confluence'],
  first_task: ['Jira', 'Stash'],
  absence_rules: ['Confluence'],
};

/** Из чего состоит заявка на доступ. */
export const ACCESS_REQUEST_FIELDS: Array<{ label: string; value: string }> = [
  { label: 'Проект / ресурс', value: 'Stash: avito-autoteka-api · Redash' },
  { label: 'Уровень прав', value: 'Чтение · Редактирование · Администрирование' },
  { label: 'Обоснование', value: 'Зачем нужен доступ и для какой задачи' },
  { label: 'Согласующий', value: 'Владелец проекта · DevOps · Руководитель' },
];

export const WEEK_OKRS: string[] = [
  'Закрыть первую боевую задачу в Stash за 2 недели',
  'Освоить пайплайн катки под фича-флагом',
  'Сформировать ИПР и трекер прогресса со 2-й недели',
];

export interface StepQuiz {
  question: string;
  options: string[];
  correct: number;
  explanation: string;
}

/** Мини-квиз: один проверочный вопрос к шагам, где стажёр изучает регламент. */
export const STEP_QUIZ: Record<string, StepQuiz> = {
  team_processes: {
    question: 'Как в команде выкатывают новую функциональность?',
    options: ['Сразу на всех пользователей', 'Под фича-флагом, после код-ревью', 'Только в конце спринта одним релизом'],
    correct: 1,
    explanation: 'Изменение закрывают фича-флагом и включают постепенно, код проходит ревью.',
  },
  absence_rules: {
    question: 'Что сделать, если нужно уйти на полдня?',
    options: ['Оформить заявку в сервисе «Отсутствия»', 'Написать в личку коллеге', 'Просто предупредить на дейли'],
    correct: 0,
    explanation: 'Отгул оформляют заявкой, её согласует руководитель, наставник получает уведомление.',
  },
};
