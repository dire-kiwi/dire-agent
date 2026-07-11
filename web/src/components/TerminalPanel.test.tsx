import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Conversation } from "../lib/protocol";
import { TerminalPanel } from "./TerminalPanel";

const mocks = vi.hoisted(() => ({
  terminalOptions: [] as Array<Record<string, unknown>>,
  addons: [] as string[],
  sockets: [] as FakeSocket[],
  fits: 0,
  focuses: 0,
  blurs: 0,
  terminalDisposals: 0,
  webglDisposals: 0,
  contextLoss: null as null | (() => void),
}));

vi.mock("@xterm/xterm", () => ({
  Terminal: class MockTerminal {
    rows = 24;
    cols = 80;
    constructor(options: Record<string, unknown>) { mocks.terminalOptions.push(options); }
    loadAddon(addon: { constructor: { name: string }; activate?: (terminal: unknown) => void }) {
      mocks.addons.push(addon.constructor.name);
      addon.activate?.(this);
    }
    open() {}
    focus() { mocks.focuses += 1; }
    blur() { mocks.blurs += 1; }
    refresh() {}
    write() {}
    writeln() {}
    clearTextureAtlas() {}
    onData() { return { dispose() {} }; }
    onResize() { return { dispose() {} }; }
    dispose() { mocks.terminalDisposals += 1; }
  },
}));

vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class MockFitAddon {
    activate() {}
    fit() { mocks.fits += 1; }
  },
}));

vi.mock("@xterm/addon-ligatures/lib/addon-ligatures.mjs", () => ({
  LigaturesAddon: class MockLigaturesAddon {
    activate() {}
  },
}));

vi.mock("@xterm/addon-webgl", () => ({
  WebglAddon: class MockWebglAddon {
    onContextLoss(listener: () => void) {
      mocks.contextLoss = listener;
      return { dispose() {} };
    }
    activate() {}
    dispose() { mocks.webglDisposals += 1; }
  },
}));

class FakeSocket {
  static OPEN = 1;
  readyState = 0;
  listeners = new Map<string, Array<(event: MessageEvent) => void>>();
  sent: string[] = [];
  closed = false;
  constructor(readonly url: string) { mocks.sockets.push(this); }
  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }
  send(value: string) { this.sent.push(value); }
  close() { this.closed = true; }
}

const project: Conversation = {
  id: "project_terminal",
  kind: "project",
  name: "Terminal project",
  cwd: "/workspace",
  model: "gpt-test",
  thinking_level: "medium",
  steering_mode: "one-at-a-time",
  follow_up_mode: "one-at-a-time",
  tools: [],
  status: "idle",
  created_at: "2026-07-11T00:00:00Z",
  updated_at: "2026-07-11T00:00:00Z",
};

describe("TerminalPanel", () => {
  beforeEach(() => {
    mocks.terminalOptions.length = 0;
    mocks.addons.length = 0;
    mocks.sockets.length = 0;
    mocks.fits = 0;
    mocks.focuses = 0;
    mocks.blurs = 0;
    mocks.terminalDisposals = 0;
    mocks.webglDisposals = 0;
    mocks.contextLoss = null;
    vi.stubGlobal("WebSocket", FakeSocket);
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("uses WebGL custom glyphs and preserves one PTY while its tab is hidden", () => {
    const launcher = { id: "nvim", label: "nvim", kind: "terminal" as const, command: "nvim", args: ["."] };
    const view = render(<TerminalPanel endpoint="ws://127.0.0.1:7331/ws" project={project} launcher={launcher} active />);

    expect(mocks.terminalOptions[0]).toMatchObject({ customGlyphs: true, letterSpacing: 0, fontFamily: expect.stringContaining("Cascadia Code") });
    expect(mocks.addons).toEqual(["MockFitAddon", "MockLigaturesAddon", "MockWebglAddon"]);
    expect(mocks.sockets).toHaveLength(1);
    expect(mocks.sockets[0].url).toContain("launcher_id=nvim");
    expect(screen.getByRole("tabpanel", { name: "nvim for Terminal project" })).toHaveClass("terminal-panel-active");

    view.rerender(<TerminalPanel endpoint="ws://127.0.0.1:7331/ws" project={project} launcher={{ ...launcher }} active={false} />);
    expect(screen.getByRole("tabpanel", { hidden: true })).not.toHaveClass("terminal-panel-active");
    expect(mocks.sockets).toHaveLength(1);
    expect(mocks.sockets[0].closed).toBe(false);

    view.rerender(<TerminalPanel endpoint="ws://127.0.0.1:7331/ws" project={project} launcher={launcher} active />);
    expect(mocks.sockets).toHaveLength(1);
    view.unmount();
    expect(mocks.sockets[0].closed).toBe(true);
    expect(mocks.terminalDisposals).toBe(1);
  });

  it("falls back to the DOM renderer after WebGL context loss", () => {
    render(<TerminalPanel endpoint="ws://127.0.0.1:7331/ws" project={project} launcher={{ id: "shell", label: "Terminal", kind: "terminal" }} active />);
    const host = screen.getByRole("application", { name: "shell terminal" });
    expect(host).toHaveAttribute("data-renderer", "webgl");
    act(() => mocks.contextLoss?.());
    expect(host).toHaveAttribute("data-renderer", "dom");
    expect(mocks.webglDisposals).toBe(1);
  });
});
