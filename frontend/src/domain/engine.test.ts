import { describe, expect, it } from "vitest";
import {
  CODEX_ENGINE,
  migrateEngineIdentity,
  NATIVE_PI_ENGINE,
} from "./engine";

describe("engine identity migration", () => {
  it("maps historical OMP labels to native PI", () => {
    for (const value of ["pi", "OMP", "oh-my-pi", "PI / oh-my-pi"]) {
      expect(migrateEngineIdentity(value, CODEX_ENGINE)).toBe(NATIVE_PI_ENGINE);
    }
    expect(migrateEngineIdentity(" Codex ", NATIVE_PI_ENGINE)).toBe(
      CODEX_ENGINE,
    );
  });

  it("preserves unknown imported identities so callers fail closed", () => {
    expect(migrateEngineIdentity("future-engine", CODEX_ENGINE)).toBe(
      "future-engine",
    );
    expect(migrateEngineIdentity(undefined, CODEX_ENGINE)).toBe(CODEX_ENGINE);
  });
});
