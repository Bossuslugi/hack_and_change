import type { ButtonHTMLAttributes, ReactNode } from 'react';
import type { Inline } from '../../lib/richText';
import { parseRichText } from '../../lib/richText';
import { Icon } from './icons';

export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}

/** Единое кольцо фокуса для всех интерактивных элементов. */
export const FOCUS = 'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-500';

export type Tone = 'neutral' | 'accent' | 'ok' | 'ai' | 'warn' | 'danger';

const BADGE_TONE: Record<Tone, string> = {
  neutral: 'bg-canvas text-ink-500',
  accent: 'bg-accent-50 text-accent-700',
  ok: 'bg-ok-50 text-ok-700',
  ai: 'bg-ai-50 text-ai-700',
  warn: 'bg-warn-50 text-warn-700',
  danger: 'bg-danger-50 text-danger-700',
};

interface BadgeProps {
  tone?: Tone;
  mono?: boolean;
  children: ReactNode;
  className?: string;
  title?: string;
}

export function Badge({ tone = 'neutral', mono = false, children, className, title }: BadgeProps) {
  return (
    <span
      title={title}
      className={cn(
        'inline-flex min-h-6 items-center gap-1 rounded-tag px-2 py-1 text-meta font-medium',
        mono && 'font-mono',
        BADGE_TONE[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

/** Плашка синтетических данных — обязательна у каждой тестовой инструкции. */
export function SyntheticBadge({ text }: { text: string }) {
  return (
    <Badge tone="warn" title="Не является регламентом Авито">
      <Icon name="warning" className="h-3 w-3" />
      {text}
    </Badge>
  );
}

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'ai';

const BUTTON_VARIANT: Record<ButtonVariant, string> = {
  primary: 'bg-accent-500 text-white hover:bg-accent-600 disabled:bg-ink-400',
  secondary: 'border border-line bg-white text-ink-900 hover:bg-canvas disabled:text-ink-400',
  ghost: 'text-ink-500 hover:bg-canvas hover:text-ink-900 disabled:text-ink-400',
  danger: 'border border-danger-500/30 bg-white text-danger-700 hover:bg-danger-50 disabled:text-ink-400',
  ai: 'bg-ai-50 text-ai-700 hover:bg-ai-500/20 disabled:text-ink-400',
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
        'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap font-medium transition-colors disabled:cursor-not-allowed',
        size === 'sm' ? 'h-8 rounded-tag px-2 text-meta' : 'h-10 rounded-card px-4 text-body',
        FOCUS,
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
    <svg viewBox="0 0 24 24" className={cn('h-4 w-4 shrink-0 animate-spin', className)} role="status" aria-label="Загрузка">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeOpacity=".2" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

/** Карточка внутри панели: белая поверхность, тонкий бордер, мягкая тень. */
export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn('rounded-card border border-line bg-white shadow-card', className)}>{children}</section>;
}

export function CardTitle({ children, aside }: { children: ReactNode; aside?: ReactNode }) {
  return (
    <div className="flex min-h-6 items-center justify-between gap-4">
      <h2 className="text-body font-semibold text-ink-900">{children}</h2>
      {aside}
    </div>
  );
}

/** Подпись раздела: моноширинный верхний регистр для технических меток. */
export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return <p className={cn('font-mono text-meta uppercase tracking-wider text-ink-400', className)}>{children}</p>;
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('skeleton', className)} aria-hidden="true" />;
}

export function EmptyState({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="rounded-card border border-dashed border-line bg-canvas px-4 py-6 text-center">
      <p className="text-body font-medium text-ink-900">{title}</p>
      {hint && <p className="mx-auto mt-1 max-w-md text-meta text-ink-500">{hint}</p>}
      {action && <div className="mt-4 flex justify-center">{action}</div>}
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
    <div className={cn('space-y-2 text-body', className)}>
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
          <List key={i} className={cn('space-y-1 pl-6 marker:text-ink-400', block.type === 'ordered' ? 'list-decimal' : 'list-disc')}>
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
      <Eyebrow className="mb-1">{label}</Eyebrow>
      <pre className="scroll-thin max-h-48 overflow-auto rounded-tag border border-line bg-canvas p-2 font-mono text-meta text-ink-900">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}
