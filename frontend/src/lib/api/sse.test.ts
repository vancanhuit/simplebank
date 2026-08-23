import { describe, expect, it, vi } from "vitest";
import { consumeEventStream } from "./sse";

const encoder = new TextEncoder();

function streamResponse(chunks: string[]): Response {
  return new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    }),
  );
}

describe("consumeEventStream", () => {
  it("parses comments, CRLF delimiters, and multiline data across chunks", async () => {
    const onEvent = vi.fn();
    const response = streamResponse([
      ": connected\r\nev",
      "ent: notification\r\ndata: first\r",
      "\ndata: second\r\nid: event-1\r\n\r",
      "\n",
    ]);

    await consumeEventStream(response, onEvent);

    expect(onEvent).toHaveBeenCalledOnce();
    expect(onEvent).toHaveBeenCalledWith({
      event: "notification",
      data: "first\nsecond",
      id: "event-1",
    });
  });

  it("dispatches a final event at EOF and defaults its type to message", async () => {
    const onEvent = vi.fn();

    await consumeEventStream(streamResponse(["data: final"]), onEvent);

    expect(onEvent).toHaveBeenCalledWith({ event: "message", data: "final", id: "" });
  });

  it("rejects on abort without invoking further callbacks", async () => {
    let enqueue!: (chunk: Uint8Array) => void;
    const response = new Response(
      new ReadableStream<Uint8Array>({
        start(controller) {
          enqueue = (chunk) => controller.enqueue(chunk);
        },
      }),
    );
    const controller = new AbortController();
    const onEvent = vi.fn(() => controller.abort());
    const consuming = consumeEventStream(response, onEvent, controller.signal);

    enqueue(encoder.encode("data: first\n\ndata: second\n\n"));

    await expect(consuming).rejects.toMatchObject({ name: "AbortError" });
    expect(onEvent).toHaveBeenCalledOnce();
  });

  it("rejects when the response has no body", async () => {
    await expect(consumeEventStream(new Response(null), vi.fn())).rejects.toThrow(
      "Notification stream has no response body",
    );
  });
});
