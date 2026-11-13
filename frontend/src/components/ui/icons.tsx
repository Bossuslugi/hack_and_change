import type { SVGProps } from 'react';

// Собственный набор линейных иконок 20×20: один вес штриха на весь интерфейс, без эмодзи.
const PATHS = {
  bolt: 'M11 2 4.5 11H9l-1 7 6.5-9H10l1-7Z',
  key: 'M6.5 16.5a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM8.8 11.2 16.5 3.5M13.5 6.5l2 2M11 9l1.5 1.5',
  shield: 'M10 2.5 4 5v4.5c0 3.6 2.4 6.6 6 8 3.6-1.4 6-4.4 6-8V5l-6-2.5Z',
  book: 'M4 4.5A1.5 1.5 0 0 1 5.5 3H16v12H5.5A1.5 1.5 0 0 0 4 16.5v-12ZM4 16.5A1.5 1.5 0 0 0 5.5 18H16M8 7h5',
  calendar: 'M4 6.5A1.5 1.5 0 0 1 5.5 5h9A1.5 1.5 0 0 1 16 6.5v9a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 4 15.5v-9ZM4 9h12M7 3v3M13 3v3',
  arrowRight: 'M4 10h12M11 5l5 5-5 5',
  arrowUp: 'M10 16V4M5 9l5-5 5 5',
  check: 'm4.5 10.5 3.5 3.5 7.5-8',
  close: 'M5 5l10 10M15 5 5 15',
  chevron: 'm5 7.5 5 5 5-5',
  sliders: 'M3 6h8M15 6h2M3 14h2M9 14h8M13 4v4M7 12v4',
  user: 'M10 10a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7ZM3.5 17c.8-3 3.4-4.5 6.5-4.5s5.7 1.5 6.5 4.5',
  mentor: 'M2 8l8-4 8 4-8 4-8-4ZM5.5 10v3.5c1.2 1.2 2.8 1.8 4.5 1.8s3.3-.6 4.5-1.8V10M18 8v4.5',
  chart: 'M3.5 16.5h13M6 13.5V10M10 13.5V5.5M14 13.5V8',
  reset: 'M4 10a6 6 0 1 0 2-4.5M4 3.5v3h3',
  warning: 'M10 3 18 16.5H2L10 3ZM10 8.5v3.5M10 14.5v.1',
  block: 'M10 17a7 7 0 1 0 0-14 7 7 0 0 0 0 14ZM5.2 5.2l9.6 9.6',
  clock: 'M10 17a7 7 0 1 0 0-14 7 7 0 0 0 0 14ZM10 6v4l2.5 2',
  doc: 'M6 2.5h5.5L15 6v11.5H6v-15ZM11 2.5V6.5h4M8 10h5M8 13h5',
  spark: 'M10 2.5c.6 4.2 3.3 6.9 7.5 7.5-4.2.6-6.9 3.3-7.5 7.5-.6-4.2-3.3-6.9-7.5-7.5 4.2-.6 6.9-3.3 7.5-7.5Z',
  info: 'M10 17a7 7 0 1 0 0-14 7 7 0 0 0 0 14ZM10 9v4.5M10 6.5v.1',
  lock: 'M5.5 9V6.5a4.5 4.5 0 0 1 9 0V9M4.5 9h11v8h-11V9ZM10 12v2',
  arrowUpRight: 'M6 14 14 6M7.5 6H14v6.5',
  activity: 'M2.5 10h3l2-5.5 3.5 11 2.5-5.5h4',
} as const;

export type IconName = keyof typeof PATHS;

interface IconProps extends SVGProps<SVGSVGElement> {
  name: IconName;
}

export function Icon({ name, className, ...rest }: IconProps) {
  return (
    <svg
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
      className={['h-4 w-4 shrink-0', className].filter(Boolean).join(' ')}
    >
      <path d={PATHS[name]} />
    </svg>
  );
}
