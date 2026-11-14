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
  team_processes: ['Confluence'],
  first_task: ['Jira', 'Stash'],
  absence_rules: ['Confluence'],
};

export interface SpecRow {
  label: string;
  value: string;
}

interface StepContext {
  stepId: string;
  week: number;
  description: string;
  stack: string;
  repository: string;
  mentorName: string;
  taskKey: string;
}

const isMl = (stack: string) => stack.includes('ML');

/** Паспорт активного шага: четыре строки о том, где шаг живёт и кто его согласует. */
export function stepSpec(ctx: StepContext): SpecRow[] {
  if (ctx.stepId === 'access_repo') {
    return [
      { label: 'Проект / ресурс', value: `Stash: ${ctx.repository} · Redash` },
      { label: 'Уровень прав', value: 'Чтение · Редактирование · Администрирование' },
      { label: 'Обоснование', value: 'Зачем нужен доступ и для какой задачи' },
      { label: 'Согласующий', value: 'Владелец проекта · DevOps · Руководитель' },
    ];
  }
  if (ctx.stepId === 'first_task') {
    return isMl(ctx.stack)
      ? [
          { label: 'Тикет и сервис', value: `Jira: ${ctx.taskKey} • Stash: ${ctx.repository}` },
          { label: 'Метрики и аналитика', value: 'Redash • Grafana (Precision/Recall и сплит A/B-теста)' },
          { label: 'Условие выкатки', value: 'Runtime Feature Flag (guard_dup_classifier_v2)' },
          { label: 'Согласующий ревьюер', value: `${ctx.mentorName} (Senior ML) • Команда Модерации` },
        ]
      : [
          { label: 'Тикет и сервис', value: `Jira: ${ctx.taskKey} • Stash: ${ctx.repository}` },
          { label: 'Метрики и аналитика', value: 'Grafana (ошибки и время ответа ручки отчёта)' },
          { label: 'Условие выкатки', value: 'Runtime Feature Flag, включение ступенями' },
          { label: 'Согласующий ревьюер', value: `${ctx.mentorName} • дежурный ревьюер команды` },
        ];
  }
  return [
    { label: 'Сервисы', value: (STEP_SERVICES[ctx.stepId] ?? ['Гоффи']).join(' • ') },
    { label: 'Срок', value: `Неделя ${ctx.week}` },
    { label: 'Что сделать', value: ctx.description },
    { label: 'Кто поможет', value: `Гоффи • ${ctx.mentorName}` },
  ];
}

export interface Okr {
  text: string;
  /** Шаги чек-листа, по которым считается прогресс цели. */
  steps: string[];
}

const GO_OKRS: Okr[] = [
  { text: 'Закрыть первую боевую задачу в Stash за 2 недели', steps: ['access_repo', 'first_task'] },
  { text: 'Освоить пайплайн катки под фича-флагом', steps: ['team_processes'] },
  { text: 'Сформировать ИПР и трекер прогресса со 2-й недели', steps: ['first_one_on_one'] },
];

const ML_OKRS: Okr[] = [
  { text: 'Выкатить классификатор дубликатов под runtime-флагом', steps: ['first_task'] },
  { text: 'Запустить A/B-тест и собрать Precision/Recall в Redash', steps: ['team_processes', 'first_task'] },
  { text: 'Согласовать дизайн эксперимента на 1:1 с наставником', steps: ['first_one_on_one'] },
];

/** Цели на адаптацию: у ML-стажёра третьей недели они про выкатку модели, у Go-стажёра — про первую задачу. */
export function okrsFor(stack: string): Okr[] {
  return isMl(stack) ? ML_OKRS : GO_OKRS;
}

const STATUS_WEIGHT: Record<string, number> = { completed: 1, in_progress: 0.5, pending: 0 };

/** Прогресс цели в процентах по статусам связанных шагов чек-листа. */
export function okrProgress(okr: Okr, checklist: Array<{ id: string; status: string }>): number {
  const linked = checklist.filter((item) => okr.steps.includes(item.id));
  if (linked.length === 0) return 0;
  const sum = linked.reduce((acc, item) => acc + (STATUS_WEIGHT[item.status] ?? 0), 0);
  return Math.round((sum / linked.length) * 100);
}

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
