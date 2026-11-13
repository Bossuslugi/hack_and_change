// Zod-схемы контрактов REST API бэкенда. Каждый ответ сервера валидируется по этим схемам
// в src/api/client.ts; типы интерфейса выводятся из схем (единый источник истины).
import { z } from 'zod';

/** Массив, который сервер может прислать как null: нормализуется в []. */
const list = <T extends z.ZodTypeAny>(item: T) =>
  z
    .array(item)
    .nullish()
    .transform((value) => value ?? []);

/** Произвольный JSON-объект (аргументы и результаты MCP), null → {}. */
const record = z
  .record(z.unknown())
  .nullish()
  .transform((value) => value ?? {});

export const roleSchema = z.enum(['intern_backend', 'mentor', 'team_lead', 'system_cron']);
export type ServerRole = z.infer<typeof roleSchema>;

export const checklistStatusSchema = z.enum(['pending', 'in_progress', 'completed']);
export type ChecklistStatus = z.infer<typeof checklistStatusSchema>;

export const checklistItemSchema = z.object({
  id: z.string(),
  position: z.number(),
  week: z.number(),
  title: z.string(),
  description: z.string(),
  status: checklistStatusSchema,
  scenario: z.string().optional(),
  evidence: z.string().optional(),
  updated_at: z.string(),
  completed_at: z.string().nullish(),
});
export type ChecklistItem = z.infer<typeof checklistItemSchema>;

export const progressSchema = z.object({
  completed: z.number(),
  in_progress: z.number(),
  total: z.number(),
  percent: z.number(),
});
export type Progress = z.infer<typeof progressSchema>;

export const docRefSchema = z.object({
  id: z.string(),
  title: z.string(),
  summary: z.string(),
  steps: list(z.string()),
  source: z.string(),
  // Жёсткое требование кейса: у каждой инструкции есть флаг синтетики и дисклеймер.
  is_synthetic: z.boolean(),
  disclaimer: z.string(),
});
export type DocRef = z.infer<typeof docRefSchema>;

export const actionCardSchema = z.object({
  type: z.string(),
  title: z.string(),
  description: z.string(),
  tool: z.string(),
  fields: record,
  submit_label: z.string(),
  status: z.enum(['pending', 'submitted']),
});
export type ActionCard = z.infer<typeof actionCardSchema>;

export const resultCardSchema = z.object({
  type: z.string(),
  status: z.enum(['success', 'denied', 'info']),
  title: z.string(),
  lines: list(z.object({ label: z.string(), value: z.string() })),
  footnote: z.string().optional(),
});
export type ResultCard = z.infer<typeof resultCardSchema>;

export const chatMessageSchema = z.object({
  id: z.string(),
  intern_id: z.string(),
  role: z.enum(['user', 'assistant']),
  text: z.string(),
  intent: z.string().optional(),
  engine: z.string().optional(),
  sources: list(docRefSchema),
  action_card: actionCardSchema.nullish(),
  result_card: resultCardSchema.nullish(),
  suggestions: list(z.string()),
  proactive: z.boolean(),
  tool_call_ids: list(z.string()),
  created_at: z.string(),
});
export type ChatMessage = z.infer<typeof chatMessageSchema>;

export const rbacVerdictSchema = z.object({
  allowed: z.boolean(),
  code: z.enum(['ALLOWED', 'FORBIDDEN']),
  http_status: z.number(),
  role: z.string(),
  operation: z.string(),
  policy_id: z.string(),
  reason: z.string(),
  constraint: z.string().optional(),
  safe_alternative: z.string().optional(),
  incident: z.boolean(),
});
export type RbacVerdict = z.infer<typeof rbacVerdictSchema>;

export const toolCallSchema = z.object({
  id: z.string(),
  intern_id: z.string(),
  actor_id: z.string(),
  actor_role: z.string(),
  server: z.string(),
  tool: z.string(),
  arguments: record,
  result: record,
  status: z.enum(['success', 'forbidden', 'invalid', 'error']),
  error_code: z.string().optional(),
  error: z.string().optional(),
  rbac: rbacVerdictSchema,
  duration_ms: z.number(),
  created_at: z.string(),
});
export type ToolCall = z.infer<typeof toolCallSchema>;

export const chatResultSchema = z.object({
  user_message: chatMessageSchema.nullish(),
  message: chatMessageSchema.nullish(),
  tool_calls: list(toolCallSchema),
  rbac: rbacVerdictSchema.nullish(),
  checklist: list(checklistItemSchema),
  checklist_updates: list(checklistItemSchema),
  progress: progressSchema,
  next_step: checklistItemSchema.nullish(),
  latency_ms: z.number(),
});
export type ChatResult = z.infer<typeof chatResultSchema>;

export const internSchema = z.object({
  id: z.string(),
  name: z.string(),
  role: z.string(),
  team: z.string(),
  stack: z.string(),
  stage: z.string(),
  program: z.string(),
  start_date: z.string(),
  repository: z.string(),
  mentor_id: z.string(),
  mentor_name: z.string(),
  team_lead_id: z.string(),
  team_lead_name: z.string(),
  is_synthetic: z.boolean(),
});
export type Intern = z.infer<typeof internSchema>;

export const sprintTaskSchema = z.object({
  key: z.string(),
  intern_id: z.string(),
  title: z.string(),
  summary: z.string(),
  status: z.string(),
  commits: z.number(),
  days_in_progress: z.number(),
  repository: z.string(),
  architecture_question: z.string(),
});
export type SprintTask = z.infer<typeof sprintTaskSchema>;

export const featureFlagSchema = z.object({
  key: z.string(),
  enabled: z.boolean(),
  kind: z.enum(['runtime', 'compile_time']),
  description: z.string(),
});
export type FeatureFlag = z.infer<typeof featureFlagSchema>;

export const metricsSchema = z.object({
  questions_answered: z.number(),
  actions_automated: z.number(),
  rbac_blocks: z.number(),
  briefs_prepared: z.number(),
  unanswered_questions: z.number(),
  tool_calls: z.number(),
  docs_read: z.number(),
  docs_total: z.number(),
  checklist_percent: z.number(),
  mentor_minutes_saved: z.number(),
  mentor_hours_baseline_min: z.number(),
  mentor_hours_baseline_max: z.number(),
  saving_percent: z.number(),
  avg_response_ms: z.number(),
});
export type Metrics = z.infer<typeof metricsSchema>;

export const notificationSchema = z.object({
  id: z.string(),
  recipient_id: z.string(),
  title: z.string(),
  body: z.string(),
  kind: z.string(),
  created_at: z.string(),
});
export type Notification = z.infer<typeof notificationSchema>;

export const briefSchema = z.object({
  id: z.string(),
  intern_id: z.string(),
  intern_name: z.string(),
  mentor_id: z.string(),
  task_key: z.string(),
  learned_topics: list(z.string()),
  architecture_question: z.string(),
  signals: list(z.string()),
  open_questions: list(z.string()),
  created_at: z.string(),
});
export type Brief = z.infer<typeof briefSchema>;

export const absenceSchema = z.object({
  id: z.string(),
  number: z.string(),
  intern_id: z.string(),
  intern_name: z.string(),
  date: z.string(),
  time_from: z.string(),
  time_to: z.string(),
  reason: z.string(),
  status: z.string(),
  notified: list(z.string()),
  is_synthetic: z.boolean(),
  created_at: z.string(),
});
export type Absence = z.infer<typeof absenceSchema>;

export const accessGrantSchema = z.object({
  id: z.string(),
  intern_id: z.string(),
  repository: z.string(),
  access_level: z.string(),
  stand: z.string(),
  token_masked: z.string(),
  expires_at: z.string(),
  is_synthetic: z.boolean(),
  created_at: z.string(),
});
export type AccessGrant = z.infer<typeof accessGrantSchema>;

export const telemetryEventSchema = z.object({
  id: z.string(),
  intern_id: z.string(),
  actor_id: z.string(),
  actor_role: z.string(),
  kind: z.string(),
  name: z.string(),
  severity: z.enum(['info', 'warn', 'security']),
  payload: record,
  created_at: z.string(),
});
export type TelemetryEvent = z.infer<typeof telemetryEventSchema>;

export const workspaceSchema = z.object({
  viewer: z.object({
    id: z.string(),
    name: z.string(),
    role: roleSchema,
    role_title: z.string(),
    team: z.string(),
  }),
  intern: internSchema,
  checklist: list(checklistItemSchema),
  progress: progressSchema,
  next_step: checklistItemSchema.nullish(),
  sprint_task: sprintTaskSchema,
  feature_flags: list(featureFlagSchema),
  metrics: metricsSchema,
  messages: list(chatMessageSchema),
  notifications: list(notificationSchema),
  briefs: list(briefSchema),
  absences: list(absenceSchema),
  access_grants: list(accessGrantSchema),
  incidents: list(telemetryEventSchema),
  doc_progress: z.object({ read: list(z.string()), total: z.number() }),
  engine: z.object({ mode: z.enum(['smart', 'llm']), model: z.string() }),
  storage: z.object({ store: z.string(), cache: z.string() }),
  permissions: z.record(z.boolean()),
  disclaimer: z.string(),
});
export type Workspace = z.infer<typeof workspaceSchema>;

export const cronResultSchema = z.object({
  triggered: z.boolean(),
  deduplicated: z.boolean(),
  reason: z.string(),
  task: sprintTaskSchema.nullish(),
  proactive_message: chatMessageSchema.nullish(),
  brief: briefSchema.nullish(),
  tool_calls: list(toolCallSchema),
  checklist: list(checklistItemSchema),
  progress: progressSchema,
});
export type CronResult = z.infer<typeof cronResultSchema>;

export const toolInfoSchema = z.object({
  name: z.string(),
  server: z.string(),
  title: z.string(),
  description: z.string(),
  side_effects: z.string(),
  requires_confirmation: z.boolean(),
  input_schema: record,
  allowed_for_viewer: z.boolean(),
});
export type ToolInfo = z.infer<typeof toolInfoSchema>;

export const telemetrySchema = z.object({
  events: list(telemetryEventSchema),
  tool_calls: list(toolCallSchema),
  tools: list(toolInfoSchema),
  servers: list(z.object({ id: z.string(), title: z.string(), description: z.string() })),
  roles: list(z.object({ id: z.string(), title: z.string(), kind: z.string() })),
  matrix: list(z.object({ operation: z.string(), access: z.record(z.boolean()) })),
});
export type Telemetry = z.infer<typeof telemetrySchema>;

export const apiErrorSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    rbac: rbacVerdictSchema.nullish(),
  }),
});

export const stageSchema = z.object({
  kind: z.string(),
  label: z.string(),
  tool: z.string().optional(),
});
export type Stage = z.infer<typeof stageSchema>;

export const streamErrorSchema = z.object({
  status: z.number(),
  error: apiErrorSchema.shape.error,
});
