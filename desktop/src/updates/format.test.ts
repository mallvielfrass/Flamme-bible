import { describe, expect, it } from "vitest";
import { formatSize, shortHash } from "./format";

describe("format", () => {
  it("formats sizes the way the download page does", () => {
    expect(formatSize(0)).toBe("");
    expect(formatSize(12)).toBe("12 Б");
    expect(formatSize(1536)).toBe("1.5 КБ");
    expect(formatSize(18 * 1024 * 1024)).toBe("18 МБ");
  });

  it("shortens a sha-256", () => {
    const hash = "ab".repeat(32);
    expect(shortHash(hash)).toBe(`${"ab".repeat(16)}…`);
    expect(shortHash("  ABC  ")).toBe("abc");
  });
});
