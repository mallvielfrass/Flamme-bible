import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";

type Reply = { status: number; body: string; chunks: number };
type Sse = { id: string; status?: number; chunk?: string };

/**
 * fetch для чата. Запрос выполняет Rust: у окна нет CORS до сервера,
 * токен в страницу не попадает. Поле chunks — сколько кусков события
 * `ai-sse` уже отправлено; окно может получить их чуть позже ответа команды.
 */
export const desktopFetch: typeof fetch = async (input, init) => {
  const path = requestPath(input);
  const method = (init?.method || "GET").toUpperCase();
  const body = typeof init?.body === "string" ? init.body : null;
  if (acceptsStream(init?.headers)) {
    return openStream(path, method, body, init?.signal ?? undefined);
  }
  const result = await invoke<Reply>("ai_http", { method, path, body });
  return jsonResponse(result);
};

function requestPath(input: RequestInfo | URL): string {
  if (typeof input === "string") return input;
  if (input instanceof URL) return `${input.pathname}${input.search}`;
  return input.url;
}

export function acceptsStream(headers: HeadersInit | undefined): boolean {
  if (!headers) return false;
  if (headers instanceof Headers) return (headers.get("accept") || "").includes("text/event-stream");
  if (Array.isArray(headers)) {
    const found = headers.find(([key]) => key.toLowerCase() === "accept");
    return Boolean(found && found[1].includes("text/event-stream"));
  }
  const record = headers as Record<string, string>;
  return (record.Accept || record.accept || "").includes("text/event-stream");
}

function jsonResponse(result: Reply): Response {
  return new Response(result.body || "", {
    status: result.status || 502,
    headers: { "content-type": "application/json" },
  });
}

async function openStream(
  path: string,
  method: string,
  body: string | null,
  signal?: AbortSignal,
): Promise<Response> {
  if (signal?.aborted) throw abortError();
  const id = crypto.randomUUID();
  const queued: string[] = [];
  let got = 0;
  let status = 0;
  let push: ((text: string | null) => void) | null = null;
  let finished = false;
  let resolveStatus: (value: number) => void = () => {};
  const statusReady = new Promise<number>((resolve) => {
    resolveStatus = resolve;
  });
  const unlisten: UnlistenFn = await listen<Sse>("ai-sse", (event) => {
    const payload = event.payload;
    if (!payload || payload.id !== id) return;
    if (payload.status) {
      status = payload.status;
      resolveStatus(payload.status);
    }
    if (!payload.chunk) return;
    got += 1;
    if (push) push(payload.chunk);
    else queued.push(payload.chunk);
  });

  const pending = invoke<Reply>("ai_http_stream", { id, method, path, body });
  const onAbort = () => {
    void invoke("ai_http_cancel", { id });
  };
  signal?.addEventListener("abort", onAbort);

  function cleanup() {
    signal?.removeEventListener("abort", onAbort);
    unlisten();
  }

  function closeStream() {
    finished = true;
    push?.(null);
  }

  let early: Reply | null = null;
  let code = 0;
  try {
    const winner = await Promise.race([
      statusReady.then((value) => ({ kind: "status" as const, value })),
      pending.then((reply) => ({ kind: "done" as const, reply })),
    ]);
    if (winner.kind === "status") code = winner.value;
    else {
      early = winner.reply;
      code = winner.reply.status;
    }
  } catch (err) {
    cleanup();
    throw err instanceof Error ? err : new Error("Нет связи с сервером.");
  }

  if (signal?.aborted) {
    cleanup();
    throw abortError();
  }
  if (code < 200 || code >= 300) {
    const reply = early ?? (await pending);
    cleanup();
    return jsonResponse(reply);
  }

  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder();
      let closed = false;
      push = (text) => {
        if (closed) return;
        if (text === null) {
          closed = true;
          controller.close();
          return;
        }
        controller.enqueue(encoder.encode(text));
      };
      for (const text of queued.splice(0)) push(text);
      if (finished) push(null);
    },
  });

  void (early ? Promise.resolve(early) : pending)
    .then(async (reply) => {
      const start = Date.now();
      while (got < (reply.chunks || 0) && Date.now() - start < 2000) {
        await new Promise((resolve) => setTimeout(resolve, 20));
      }
      closeStream();
      cleanup();
    })
    .catch(() => {
      closeStream();
      cleanup();
    });

  return new Response(stream, {
    status: code || status || 200,
    headers: { "content-type": "text/event-stream" },
  });
}

function abortError(): Error {
  const error = new Error("The operation was aborted");
  error.name = "AbortError";
  return error;
}
