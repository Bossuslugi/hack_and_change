// Разбор потока Server-Sent Events, полученного через fetch (EventSource не умеет POST).

export type SseHandler = (event: string, data: unknown) => void;

/** Разбирает один блок SSE («event: …\ndata: …») и вызывает обработчик. */
export function dispatchSseBlock(block: string, handler: SseHandler): void {
  let event = 'message';
  const dataLines: string[] = [];
  for (const rawLine of block.split('\n')) {
    const line = rawLine.replace(/\r$/, '');
    if (line.startsWith('event:')) event = line.slice(6).trim();
    else if (line.startsWith('data:')) dataLines.push(line.slice(5).replace(/^ /, ''));
  }
  if (dataLines.length === 0) return;
  let data: unknown;
  try {
    data = JSON.parse(dataLines.join('\n'));
  } catch {
    return; // битый фрагмент пропускаем, поток продолжается
  }
  handler(event, data);
}

/** Читает поток до конца, вызывая обработчик для каждого события. */
export async function readSse(body: ReadableStream<Uint8Array>, handler: SseHandler): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.search(/\r?\n\r?\n/);
    while (boundary !== -1) {
      const block = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary).replace(/^\r?\n\r?\n/, '');
      dispatchSseBlock(block, handler);
      boundary = buffer.search(/\r?\n\r?\n/);
    }
  }
  buffer += decoder.decode();
  if (buffer.trim() !== '') dispatchSseBlock(buffer, handler);
}
