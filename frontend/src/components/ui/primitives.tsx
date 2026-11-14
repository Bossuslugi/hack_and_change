import type { ButtonHTMLAttributes, ReactNode } from 'react';
import type { Inline } from '../../lib/richText';
import { parseRichText } from '../../lib/richText';

export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}

type Tone = 'neutral' | 'brand' | 'ok' | 'warn' | 'danger' | 'accent';

const BADGE_TONE: Record<Tone, string> = {
  neutral: 'bg-paper-100 text-ink-700',
  brand: 'bg-brand-50 text-brand-700',
  ok: 'bg-ok-50 text-ok-700',
  warn: 'bg-warn-50 text-warn-700',
  danger: 'bg-danger-50 text-danger-700',
  accent: 'bg-accent-50 text-accent-700',
};

export function Badge({ tone = 'neutral', children, className, title }: { tone?: Tone; children: ReactNode; className?: string; title?: string }) {
  return (
    <span
      title={title}
      className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium leading-4', BADGE_TONE[tone], className)}
    >
      {children}
    </span>
  );
}

/** Плашка синтетических данных — обязательна у каждой тестовой инструкции. */
export function SyntheticBadge({ text }: { text: string }) {
  return (
    <Badge tone="warn" title="Не является регламентом Авито">
      <svg viewBox="0 0 16 16" className="h-3 w-3" aria-hidden="true">
        <path d="M8 1.5 15 14H1L8 1.5Zm0 4.2v4.1m0 1.7v.6" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      {text}
    </Badge>
  );
}

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

const BUTTON_VARIANT: Record<ButtonVariant, string> = {
  primary: 'bg-brand-500 text-white hover:bg-brand-600 active:bg-brand-700 disabled:bg-ink-300',
  secondary: 'bg-paper-0 text-ink-900 ring-1 ring-inset ring-paper-200 hover:bg-paper-50 disabled:text-ink-300',
  ghost: 'text-ink-700 hover:bg-paper-100 disabled:text-ink-300',
  danger: 'bg-paper-0 text-danger-700 ring-1 ring-inset ring-danger-500/30 hover:bg-danger-50 disabled:text-ink-300',
};

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  loading?: boolean;
  size?: 'sm' | 'md';
}

export function Button({ variant = 'secondary', loading = false, size = 'md', className, children, disabled, ...rest }: ButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-xl font-medium transition-all duration-200 active:scale-[.97]',
        'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500',
        'disabled:cursor-not-allowed',
        size === 'sm' ? 'px-2.5 py-1.5 text-xs' : 'px-3.5 py-2 text-sm',
        BUTTON_VARIANT[variant],
        className,
      )}
    >
      {loading && <Spinner />}
      {children}
    </button>
  );
}

export function Spinner({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={cn('h-4 w-4 animate-spin', className)} role="status" aria-label="Загрузка">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeOpacity=".25" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn('bento', className)}>{children}</section>;
}

export function CardTitle({ children, aside }: { children: ReactNode; aside?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <h2 className="text-sm font-semibold text-ink-900">{children}</h2>
      {aside}
    </div>
  );
}

export function EmptyState({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="rounded-2xl border border-dashed border-paper-200 bg-paper-50 px-4 py-6 text-center">
      <p className="text-sm font-medium text-ink-700">{title}</p>
      {hint && <p className="mx-auto mt-1 max-w-md text-xs text-ink-500">{hint}</p>}
      {action && <div className="mt-3 flex justify-center">{action}</div>}
    </div>
  );
}

function Inlines({ inlines }: { inlines: Inline[] }) {
  return (
    <>
      {inlines.map((part, i) =>
        part.bold ? (
          <strong key={i} className="font-semibold">
            {part.text}
          </strong>
        ) : (
          <span key={i}>{part.text}</span>
        ),
      )}
    </>
  );
}

/** Рендер ответа агента: абзацы, списки, жирный. Текст вставляется только как текст. */
export function RichText({ text, className }: { text: string; className?: string }) {
  const blocks = parseRichText(text);
  return (
    <div className={cn('space-y-2 text-sm leading-relaxed', className)}>
      {blocks.map((block, i) => {
        if (block.type === 'paragraph') {
          return (
            <p key={i}>
              <Inlines inlines={block.inlines} />
            </p>
          );
        }
        const List = block.type === 'ordered' ? 'ol' : 'ul';
        return (
          <List key={i} className={cn('space-y-1 pl-5', block.type === 'ordered' ? 'list-decimal' : 'list-disc', 'marker:text-ink-400')}>
            {block.items.map((item, j) => (
              <li key={j}>
                <Inlines inlines={item} />
              </li>
            ))}
          </List>
        );
      })}
    </div>
  );
}

export function JsonBlock({ value, label }: { value: unknown; label: string }) {
  return (
    <div>
      <p className="mb-1 text-[11px] font-medium uppercase tracking-wide text-ink-400">{label}</p>
      <pre className="max-h-48 overflow-auto rounded-xl bg-[#0B0F19] p-2.5 font-mono text-[11px] leading-relaxed text-slate-200 ring-1 ring-white/5">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

/** Пульсирующая точка «онлайн». */
export function OnlineDot({ className }: { className?: string }) {
  return (
    <span className={cn('relative inline-flex h-2 w-2', className)} aria-hidden="true">
      <span className="absolute inset-0 animate-pulse-ring rounded-full bg-ok-500" />
      <span className="relative h-2 w-2 rounded-full bg-ok-500" />
    </span>
  );
}
