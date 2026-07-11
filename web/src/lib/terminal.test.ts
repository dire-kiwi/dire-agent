import { describe, expect, it } from "vitest";
import {
  defaultProjectLaunchers,
  effectiveProjectLaunchers,
  formatLauncherShortcut,
  matchesLauncherShortcut,
  terminalFallbackLigatures,
  terminalFontFamily,
  terminalIconFont,
  terminalIconProbe,
  terminalLigatureFeatures,
  terminalTheme,
  terminalWebSocketURL,
} from "./terminal";

describe("terminalWebSocketURL", () => {
  it("reuses the daemon authority and scopes the PTY to a project and launcher", () => {
    expect(terminalWebSocketURL("wss://daemon.example/ws?old=1", "project/a", "lazygit"))
      .toBe("wss://daemon.example/terminal?project_id=project%2Fa&launcher_id=lazygit");
  });

  it("keeps explicit empty launchers distinct from legacy defaults", () => {
    expect(effectiveProjectLaunchers(undefined)).toEqual(defaultProjectLaunchers);
    expect(effectiveProjectLaunchers(null)).toEqual(defaultProjectLaunchers);
    expect(effectiveProjectLaunchers([])).toEqual([]);
    const copy = effectiveProjectLaunchers(undefined);
    copy[2].args?.push("changed");
    expect(defaultProjectLaunchers[2].args).toEqual(["."]);
  });

  it("matches and formats configurable modifier shortcuts", () => {
    const event = new KeyboardEvent("keydown", { key: "G", code: "KeyG", ctrlKey: true, shiftKey: true });
    expect(matchesLauncherShortcut(event, "mod+shift+g")).toBe(true);
    expect(matchesLauncherShortcut(event, "mod+g")).toBe(false);
    expect(matchesLauncherShortcut(new KeyboardEvent("keydown", { key: "`", code: "Backquote", metaKey: true }), "mod+backquote")).toBe(true);
    expect(formatLauncherShortcut("mod+shift+g")).toBe("⌘/Ctrl + Shift + G");
  });

  it("uses Cascadia Code with a colorful terminal-safe fallback palette", () => {
    expect(terminalFontFamily.startsWith('"Cascadia Code"')).toBe(true);
    expect(terminalFontFamily).toContain(terminalIconFont);
    expect(terminalIconProbe).toContain("\ue0a0");
    expect(terminalIconProbe).toContain("\u{f0001}");
    expect(terminalTheme.background).toBe("#222436");
    expect(new Set([
      terminalTheme.red,
      terminalTheme.green,
      terminalTheme.yellow,
      terminalTheme.blue,
      terminalTheme.magenta,
      terminalTheme.cyan,
    ]).size).toBe(6);
  });

  it("enables Cascadia Code ligatures for common programming operators", () => {
    expect(terminalLigatureFeatures).toContain('"calt" on');
    expect(terminalLigatureFeatures).toContain('"liga" on');
    expect(terminalFallbackLigatures).toEqual(expect.arrayContaining([
      "->", "=>", "!=", "===", "<=", ">=", "::", ":=", "&&", "||",
    ]));
  });
});
