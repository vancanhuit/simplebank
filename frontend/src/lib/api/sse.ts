export interface ServerSentEvent {
  event: string;
  data: string;
  id: string;
}

export async function consumeEventStream(
  response: Response,
  onEvent: (event: ServerSentEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  if (!response.body) {
    throw new Error("Notification stream has no response body");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let eventType = "";
  let lastEventId = "";
  let data: string[] = [];

  const throwIfAborted = () => {
    if (signal?.aborted) {
      throw new DOMException("The operation was aborted", "AbortError");
    }
  };

  const dispatch = () => {
    if (data.length === 0) {
      eventType = "";
      return;
    }
    onEvent({
      event: eventType || "message",
      data: data.join("\n"),
      id: lastEventId,
    });
    eventType = "";
    data = [];
  };

  const processLine = (line: string) => {
    throwIfAborted();
    if (line === "") {
      dispatch();
      return;
    }
    if (line.startsWith(":")) {
      return;
    }

    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) {
      value = value.slice(1);
    }

    if (field === "event") {
      eventType = value;
    } else if (field === "data") {
      data.push(value);
    } else if (field === "id" && !value.includes("\0")) {
      lastEventId = value;
    }
  };

  const processBuffer = (atEof: boolean) => {
    while (true) {
      const lineEnd = buffer.search(/[\r\n]/);
      if (lineEnd === -1) {
        break;
      }
      if (buffer[lineEnd] === "\r" && lineEnd === buffer.length - 1 && !atEof) {
        break;
      }

      const line = buffer.slice(0, lineEnd);
      const delimiterLength = buffer.startsWith("\r\n", lineEnd) ? 2 : 1;
      buffer = buffer.slice(lineEnd + delimiterLength);
      processLine(line);
    }

    if (atEof && buffer !== "") {
      processLine(buffer);
      buffer = "";
    }
  };

  const abort = () => {
    void reader.cancel();
  };
  signal?.addEventListener("abort", abort, { once: true });

  try {
    throwIfAborted();
    while (true) {
      const { done, value } = await reader.read();
      throwIfAborted();
      if (done) {
        buffer += decoder.decode();
        processBuffer(true);
        dispatch();
        return;
      }
      buffer += decoder.decode(value, { stream: true });
      processBuffer(false);
    }
  } finally {
    signal?.removeEventListener("abort", abort);
    reader.releaseLock();
  }
}
