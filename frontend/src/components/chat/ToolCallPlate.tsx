import { useState } from 'react';
import type { ToolCall } from '../../api/schemas';
import { formatMs } from '../../lib/format';
import { Icon } from '../ui/icons';
import { FOCUS, JsonBlock, cn } from '../ui/primitives';

/** «INTERN_ALLOWED» / «INTERN_DENIED»: роль субъекта и вердикт RBAC одной меткой. */
export function rbacLabel(call: ToolCall): string {
  const subject = (call.actor_role.split('_')[0] ?? call.actor_role).toUpperCase();
  return `${subject}_${call.rbac.allowed ? 'ALLOWED' : 'DENIED'}`;
}

/** Сворачиваемая плашка MCP-вызова: показывается перед ответом агента. */
export function ToolCallPlate({ call }: { call: ToolCall }) {
  const [open, setOpen] = useState(false);
  const allowed = call.rbac.allowed;
  return (
    <div className="rounded-card border border-line bg-white" data-testid="mcp-plate">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className={cn('flex min-h-8 w-full items-center gap-2 rounded-card px-2 py-1 text-left font-mono text-meta text-ink-500', FOCUS)}
      >
        <span className="grid h-5 w-5 shrink-0 place-items-center rounded-tag bg-ai-50 text-ai-700" aria-hidden="true">
          <Icon name="bolt" className="h-3 w-3" />
        </span>
        <span className="min-w-0 flex-1 truncate">
          Вызван MCP: <span className="font-medium text-ink-900">{call.tool}</span>
          <span className="text-ink-400"> • </span>
          RBAC: <span className={cn('font-medium', allowed ? 'text-ok-700' : 'text-danger-700')}>{rbacLabel(call)}</span>
          <span className="text-ink-400"> • </span>
          {formatMs(call.duration_ms)}
        </span>
        <Icon name="chevron" className={cn('h-3 w-3 transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="space-y-2 border-t border-line p-2">
          <p className="font-mono text-meta text-ink-500">
            policy: {call.rbac.policy_id}
            {call.rbac.constraint && ` · constraint: ${call.rbac.constraint}`}
          </p>
          <p className="text-meta text-ink-500">{call.rbac.reason}</p>
          <JsonBlock label="Аргументы" value={call.arguments} />
          {call.status === 'success' && <JsonBlock label="Ответ сервиса" value={call.result} />}
        </div>
      )}
    </div>
  );
}
