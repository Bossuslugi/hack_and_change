import { useEffect, useState } from 'react';
import { cn } from './ui/primitives';

/**
 * Картинка маскота. Положите файл в frontend/public/goffy.png — он подхватится автоматически.
 * Пока файла нет, показывается встроенный векторный Гоффи.
 */
export const GOFFY_IMAGE_URL = '/goffy.png';

const SIZES = {
  sm: 'h-8 w-8',
  md: 'h-12 w-12',
  lg: 'h-24 w-24',
} as const;

export type GoffySize = keyof typeof SIZES;

let imageStatus: 'unknown' | 'ok' | 'missing' = 'unknown';

/** Проверяет наличие картинки маскота один раз за сессию. */
function useGoffyImage(): boolean {
  const [ok, setOk] = useState(imageStatus === 'ok');
  useEffect(() => {
    if (imageStatus !== 'unknown') return;
    const probe = new Image();
    probe.onload = () => {
      imageStatus = 'ok';
      setOk(true);
    };
    probe.onerror = () => {
      imageStatus = 'missing';
    };
    probe.src = GOFFY_IMAGE_URL;
  }, []);
  return ok;
}

/** Только для тестов: сбрасывает кэш проверки картинки. */
export function resetGoffyImageCache(status: 'unknown' | 'ok' | 'missing' = 'unknown'): void {
  imageStatus = status;
}

function GoffyVector() {
  return (
    <svg viewBox="0 0 64 64" className="h-full w-full" aria-hidden="true">
      <defs>
        <linearGradient id="goffy-body" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#3AC4FF" />
          <stop offset="1" stopColor="#7C5CFF" />
        </linearGradient>
      </defs>
      {/* антенна */}
      <path d="M32 12V6" stroke="#7C5CFF" strokeWidth="3" strokeLinecap="round" />
      <circle cx="32" cy="5" r="3.2" fill="#10B981" />
      {/* уши-наушники */}
      <rect x="5" y="28" width="7" height="14" rx="3.5" fill="#7C5CFF" />
      <rect x="52" y="28" width="7" height="14" rx="3.5" fill="#7C5CFF" />
      {/* голова */}
      <rect x="9" y="12" width="46" height="42" rx="17" fill="url(#goffy-body)" />
      {/* визор */}
      <rect x="15" y="21" width="34" height="20" rx="10" fill="#0B0F19" />
      {/* глаза: правый подмигивает */}
      <ellipse cx="25" cy="31" rx="3.6" ry="4.4" fill="#5EEAD4" />
      <ellipse cx="39" cy="31" rx="3.6" ry="4.4" fill="#5EEAD4" className="origin-center animate-wink" style={{ transformBox: 'fill-box' }} />
      {/* улыбка */}
      <path d="M25 46c2.2 2.6 11.8 2.6 14 0" stroke="#fff" strokeWidth="2.6" strokeLinecap="round" fill="none" />
      {/* румянец */}
      <circle cx="17" cy="45" r="2.6" fill="#fff" opacity=".28" />
      <circle cx="47" cy="45" r="2.6" fill="#fff" opacity=".28" />
    </svg>
  );
}

interface GoffyAvatarProps {
  size?: GoffySize;
  /** Пульсирующее кольцо «на связи». */
  pulse?: boolean;
  className?: string;
}

/** Аватар маскота Гоффи: картинка (если добавлена) или встроенный вектор с подмигиванием. */
export function GoffyAvatar({ size = 'sm', pulse = false, className }: GoffyAvatarProps) {
  const hasImage = useGoffyImage();
  return (
    <span className={cn('relative inline-flex shrink-0', SIZES[size], className)} role="img" aria-label="Гоффи" data-size={size}>
      {pulse && <span className="absolute inset-0 animate-pulse-ring rounded-full bg-brand-500/40" aria-hidden="true" />}
      <span className={cn('relative block h-full w-full', size !== 'sm' && 'animate-float')}>
        {hasImage ? (
          <img src={GOFFY_IMAGE_URL} alt="" className="h-full w-full rounded-full object-cover ring-2 ring-brand-500/30" />
        ) : (
          <GoffyVector />
        )}
      </span>
    </span>
  );
}
