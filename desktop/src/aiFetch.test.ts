import { describe, expect, it } from "vitest";
import { acceptsStream } from "./aiFetch";

describe("acceptsStream", () => {
  it("recognises the chat event stream header", () => {
    expect(acceptsStream({ Accept: "text/event-stream" })).toBe(true);
    expect(acceptsStream({ accept: "application/json" })).toBe(false);
    expect(acceptsStream(undefined)).toBe(false);
    expect(acceptsStream([["Accept", "text/event-stream"]])).toBe(true);
  });
});
