// Раскладка чек-листа по этапам адаптации и корпоративным сервисам.
// Сервер отдаёт плоский список шагов; группировка — задача интерфейса.
import type { ChecklistItem } from '../api/schemas';

export type StageId = 'access' | 'process' | 'org';

export const STAGES: Array<{ id: StageId; title: string }> = [
  { id: 'access', title: 'Доступы и окружение' },
  { id: 'process', title: 'Процессы и Playbook' },
  { id: 'org', title: 'Организационные вопросы' },
];

interface StepMeta {
  stage: StageId;
  /** Корпоративный сервис, в котором закрывается шаг. */
  service: string;
}

const STEP_META: Record<string, StepMeta> = {
  access_repo: { stage: 'access', service: 'GitHub / IAM' },
  local_env: { stage: 'access', service: 'Окружение' },
  team_processes: { stage: 'process', service: 'Playbook' },
  first_task: { stage: 'process', service: 'Трекер' },
  meet_team: { stage: 'org', service: 'Команда' },
  absence_rules: { stage: 'org', service: 'Отсутствия' },
  first_one_on_one: { stage: 'org', service: 'Наставничество' },
};

const SCENARIO_STAGE: Record<string, StageId> = { S1: 'access', S2: 'process', S3: 'org' };

/** Этап и сервис шага; для незнакомых шагов этап берётся из сценария. */
export function stepMeta(item: Pick<ChecklistItem, 'id' | 'scenario'>): StepMeta {
  return STEP_META[item.id] ?? { stage: SCENARIO_STAGE[item.scenario ?? ''] ?? 'org', service: 'Онбординг' };
}

export interface StageGroup {
  id: StageId;
  title: string;
  items: ChecklistItem[];
  completed: number;
}

/** Группирует шаги по этапам, сохраняя серверный порядок внутри этапа. */
export function groupByStage(items: ChecklistItem[]): StageGroup[] {
  return STAGES.map((stage) => {
    const own = items.filter((item) => stepMeta(item).stage === stage.id);
    return { ...stage, items: own, completed: own.filter((item) => item.status === 'completed').length };
  }).filter((group) => group.items.length > 0);
}

/** Запросы к AI-Buddy по шагам чек-листа (совпадают с подсказками агента). */
export const STEP_PROMPTS: Record<string, string> = {
  access_repo: 'Дай доступ к репозиторию avito-autoteka-api и тестовому стенду',
  team_processes: 'Как в нашей команде катить задачу под фича-флагом и проходить ревью?',
  first_task: 'Я застрял на первой задаче, с чего начать?',
  absence_rules: 'Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?',
  first_one_on_one: 'Как проходят встречи 1:1?',
};
