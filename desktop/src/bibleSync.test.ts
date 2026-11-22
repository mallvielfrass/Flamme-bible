import { describe, expect, it } from "vitest";
import { bibleStatus } from "./bibleSync";

describe("bible module row", () => {
  it("does not call an existing module an upload", () => {
    expect(bibleStatus(undefined)).toBe("Ещё не сверено с сервером");
    expect(bibleStatus({ server_id: "", uploaded: false })).toBe("Ещё не сверено с сервером");
    expect(bibleStatus({ server_id: "module", uploaded: false })).toBe("Уже на сервере");
    expect(bibleStatus({ server_id: "module", uploaded: true })).toBe("Отправлено на сервер");
  });
});
