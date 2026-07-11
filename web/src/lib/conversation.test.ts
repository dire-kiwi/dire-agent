import { describe, expect, it } from "vitest";
import {
  emptyConversation,
  completeSlashCommand,
  cleanReasoningText,
  hydrateMessages,
  parseComposerInput,
  reduceEvent,
  slashCommandSuggestions,
} from "./conversation";
import type { WireEvent } from "./protocol";

function event(
  sequence: number,
  type: string,
  data: Record<string, unknown> = {},
): WireEvent {
  return {
    sequence,
    type,
    thread_id: "thread_test",
    timestamp: "2026-07-10T00:00:00Z",
    data,
  };
}

describe("conversation reducer", () => {
  it("removes provider comment sentinels from reasoning summaries", () => {
    expect(cleanReasoningText("**Inspecting**\n\n<!-- -->")).toBe("**Inspecting**");
  });
  it("hydrates persisted messages and tool labels", () => {
    expect(
      hydrateMessages([
        {
          sequence: 1,
          kind: "message",
          role: "user",
          content: "hello",
          created_at: "now",
        },
        {
          sequence: 2,
          kind: "tool",
          role: "tool",
          content: "file contents",
          data: { tool_name: "read", arguments: { path: "README.md" } },
          created_at: "now",
        },
        {
          sequence: 3,
          kind: "reasoning",
          role: "reasoning",
          content: "I should inspect the file.",
          created_at: "now",
        },
      ]),
    ).toMatchObject([
      { id: "stored-1", role: "user", content: "hello" },
      { id: "stored-2", role: "tool", label: "read", arguments: { path: "README.md" } },
      { id: "stored-3", role: "reasoning", content: "I should inspect the file." },
    ]);
  });

  it("hydrates persisted project image attachments", () => {
    const messages = hydrateMessages([{
      sequence: 1,
      kind: "message",
      role: "user",
      content: "inspect",
      data: { attachments: [{ name: "paste.png", mime_type: "image/png", file: "image_1.png" }] },
      created_at: "2026-07-11T00:00:00Z",
    }], (file) => `http://daemon/attachments/project_1/${file}`);
    expect(messages[0].attachments).toEqual([{
      name: "paste.png",
      mimeType: "image/png",
      url: "http://daemon/attachments/project_1/image_1.png",
    }]);
  });

  it("assembles streaming deltas into the authoritative final message", () => {
    let state = reduceEvent(
      emptyConversation,
      event(1, "message_start", { message_id: "m1" }),
    );
    state = reduceEvent(
      state,
      event(2, "message_update", { message_id: "m1", delta: "hel" }),
    );
    state = reduceEvent(
      state,
      event(3, "message_update", { message_id: "m1", delta: "lo" }),
    );
    expect(state.stream?.content).toBe("hello");
    state = reduceEvent(
      state,
      event(4, "message_end", { message_id: "m1", text: "Hello." }),
    );
    expect(state.stream).toBeNull();
    expect(state.messages.at(-1)).toMatchObject({
      role: "assistant",
      content: "Hello.",
    });
  });

  it("tracks tools, queues, settlement, gaps, and duplicate events", () => {
    let state = reduceEvent(emptyConversation, event(3, "agent_start"));
    expect(state.running).toBe(true);
    state = reduceEvent(
      state,
      event(4, "tool_execution_start", {
        tool_call_id: "call-1",
        tool_name: "read",
        arguments: { path: "README.md" },
      }),
    );
    expect(state.activeTools).toHaveLength(1);
    state = reduceEvent(
      state,
      event(5, "tool_execution_end", {
        tool_call_id: "call-1",
        tool_name: "read",
        arguments: { path: "README.md" },
        output: "contents",
      }),
    );
    expect(state.activeTools).toHaveLength(0);
    expect(state.messages.at(-1)).toMatchObject({ role: "tool", label: "read", arguments: { path: "README.md" } });
    const duplicate = reduceEvent(
      state,
      event(5, "tool_execution_end", { output: "duplicate" }),
    );
    expect(duplicate).toBe(state);
    state = reduceEvent(
      state,
      event(7, "queue_update", { steering: 1, follow_ups: 2 }),
    );
    expect(state.hasSequenceGap).toBe(true);
    expect(state.followUpsQueued).toBe(2);
    state = reduceEvent(state, event(8, "agent_settled", { settled: true }));
    expect(state.running).toBe(false);
    expect(state.followUpsQueued).toBe(0);
  });

  it("streams and completes a visible reasoning summary", () => {
    let state = reduceEvent(emptyConversation, event(1, "reasoning_start", { message_id: "r1" }));
    state = reduceEvent(state, event(2, "reasoning_update", { message_id: "r1", delta: "Inspecting " }));
    state = reduceEvent(state, event(3, "reasoning_update", { message_id: "r1", delta: "the project." }));
    expect(state.reasoning).toEqual({ id: "r1", content: "Inspecting the project." });
    state = reduceEvent(state, event(4, "reasoning_end", { message_id: "r1", text: "Inspecting the project." }));
    expect(state.reasoning).toBeNull();
    expect(state.messages.at(-1)).toMatchObject({ role: "reasoning", content: "Inspecting the project." });
  });
});

describe("slash command parser", () => {
  it.each([
    ["hello", { kind: "prompt", value: "hello" }],
    ["/steer focus", { kind: "steer", value: "focus" }],
    ["/followup next", { kind: "follow-up", value: "next" }],
    ["/abort", { kind: "abort" }],
    ["/model gpt-next", { kind: "model", value: "gpt-next" }],
    ["/thinking HIGH", { kind: "thinking", value: "high" }],
    ["/name demo task", { kind: "name", value: "demo task" }],
    ["/folders", { kind: "folders" }],
    ["/folder-add /workspace/shared files", { kind: "folder-add", value: "/workspace/shared files" }],
    ["/folder-remove /workspace/shared", { kind: "folder-remove", value: "/workspace/shared" }],
    ["/help", { kind: "help" }],
    ["/quit", { kind: "quit" }],
  ])("parses %s", (input, expected) => {
    expect(parseComposerInput(input)).toEqual(expected);
  });

  it("rejects missing and unknown commands", () => {
    expect(() => parseComposerInput("/steer")).toThrow("requires text");
    expect(() => parseComposerInput("/nope")).toThrow("Unknown command");
  });

  it("suggests and completes canonical slash commands", () => {
    expect(slashCommandSuggestions("/thi").map((command) => command.name)).toEqual(["thinking"]);
    expect(slashCommandSuggestions("/").map((command) => command.name)).toContain("follow-up");
    expect(slashCommandSuggestions("/thinking")).toEqual([]);
    expect(slashCommandSuggestions("/thinking h")).toEqual([]);
    expect(completeSlashCommand(slashCommandSuggestions("/thi")[0])).toBe("/thinking ");
    expect(completeSlashCommand(slashCommandSuggestions("/he")[0])).toBe("/help");
  });
});
