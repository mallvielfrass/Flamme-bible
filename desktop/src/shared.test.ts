import { describe, expect, it } from "vitest";
import { AppModal, ConfirmDialog, IconButton, SpaceFilter } from "./shared";

describe("shared site ui", () => {
  it("loads the site components without mounting them", () => {
    for (const component of [AppModal, ConfirmDialog, IconButton, SpaceFilter]) {
      expect(component).toBeTruthy();
      expect(component).toHaveProperty("setup");
    }
  });
});
