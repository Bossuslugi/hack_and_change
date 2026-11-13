// Минимальный разбор разметки ответов агента: **жирный**, нумерованные и маркированные списки.
// Без dangerouslySetInnerHTML: текст модели никогда не попадает в DOM как HTML.

export interface Inline {
  text: string;
  bold: boolean;
}

export type Block =
  | { type: 'paragraph'; inlines: Inline[] }
  | { type: 'ordered' | 'unordered'; items: Inline[][] };

/** Делит строку на обычные и **жирные** фрагменты. */
export function parseInline(text: string): Inline[] {
  const out: Inline[] = [];
  const parts = text.split(/\*\*(.+?)\*\*/g);
  parts.forEach((part, index) => {
    if (part !== '') out.push({ text: part, bold: index % 2 === 1 });
  });
  return out;
}

const ORDERED = /^\d+[.)]\s+/;
const UNORDERED = /^[-•*]\s+/;

/** Превращает текст сообщения в блоки для рендера. */
export function parseRichText(text: string): Block[] {
  const blocks: Block[] = [];
  for (const rawLine of text.split('\n')) {
    const line = rawLine.trim();
    if (line === '') continue;
    const type = ORDERED.test(line) ? 'ordered' : UNORDERED.test(line) ? 'unordered' : 'paragraph';
    if (type === 'paragraph') {
      blocks.push({ type, inlines: parseInline(line) });
      continue;
    }
    const item = parseInline(line.replace(type === 'ordered' ? ORDERED : UNORDERED, ''));
    const last = blocks[blocks.length - 1];
    if (last && last.type === type) last.items.push(item);
    else blocks.push({ type, items: [item] });
  }
  return blocks;
}
