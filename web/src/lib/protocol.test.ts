import { describe, expect, it } from "vitest";
import {
  addUsage,
  conversationScope,
  normalizeRuntimeState,
  normalizeUsage,
  wireConversationID,
  wireProjectID,
  type Conversation,
  type Command,
  type RuntimeState,
} from "./protocol";

describe("usage protocol helpers", () => {
  it("normalizes token counters and derives a missing total", () => {
    expect(
      normalizeUsage({
        input_tokens: 12,
        output_tokens: 3,
        cache_read_tokens: 8,
        cache_write_tokens: 2,
        context_tokens: 11,
        context_window: 128,
      }),
    ).toEqual({
      input_tokens: 12,
      output_tokens: 3,
      cache_read_tokens: 8,
      cache_write_tokens: 2,
      total_tokens: 15,
      context_tokens: 11,
      context_window: 128,
    });
  });

  it("adds cumulative counters while using the latest context snapshot", () => {
    expect(
      addUsage(
        {
          input_tokens: 10,
          output_tokens: 2,
          cache_read_tokens: 6,
          cache_write_tokens: 1,
          total_tokens: 12,
          context_tokens: 8,
          context_window: 100,
        },
        {
          input_tokens: 4,
          output_tokens: 1,
          cache_read_tokens: 3,
          cache_write_tokens: 0,
          total_tokens: 5,
          context_tokens: 13,
          context_window: 100,
        },
      ),
    ).toEqual({
      input_tokens: 14,
      output_tokens: 3,
      cache_read_tokens: 9,
      cache_write_tokens: 1,
      total_tokens: 17,
      context_tokens: 13,
      context_window: 100,
    });
  });

  it("prefers project_id but accepts legacy thread events", () => {
    const base = { type: "agent_start", timestamp: "now" };
    expect(wireProjectID({ ...base, project_id: "project_1", thread_id: "thread_1" })).toBe("project_1");
    expect(wireProjectID({ ...base, thread_id: "thread_1" })).toBe("thread_1");
  });

  it("routes generic, chat, project, scope, and legacy event identifiers", () => {
    const base = { type: "agent_start", timestamp: "now" };
    expect(wireConversationID({ ...base, conversation_id: "conversation_1", chat_id: "chat_1" })).toBe("conversation_1");
    expect(wireConversationID({ ...base, chat_id: "chat_1", project_id: "project_1" })).toBe("chat_1");
    expect(wireConversationID({ ...base, scope: { kind: "chat", id: "chat_scope" } })).toBe("chat_scope");
    expect(conversationScope({ id: "chat_1", kind: "chat" })).toEqual({
      conversation_id: "chat_1",
      chat_id: "chat_1",
      thread_id: "chat_1",
    });
  });

  it("ignores zero-value project objects when normalizing chat state", () => {
    const chat: Conversation = {
      id: "chat_1", kind: "chat", name: "Chat", model: "gpt-test", cwd: "",
      thinking_level: "medium", steering_mode: "one-at-a-time", follow_up_mode: "one-at-a-time",
      tools: [], status: "idle", created_at: "now", updated_at: "now",
    };
    const emptyProject = { ...chat, id: "", kind: "project" as const };
    const raw = {
      kind: "chat",
      conversation: chat,
      chat,
      project: emptyProject,
      thread: chat,
      running: false,
      steering_queued: 0,
      follow_ups_queued: 0,
    } as RuntimeState;
    const normalized = normalizeRuntimeState(raw, chat);
    expect(normalized.conversation.id).toBe("chat_1");
    expect(normalized.project).toBeUndefined();
    expect(normalized.chat?.id).toBe("chat_1");
  });

  it("models capability command names and free-form arguments on the wire", () => {
    const command: Command = {
      type: "execute_capability_command",
      conversation_id: "chat_1",
      command_name: "release",
      arguments: "v2.0 --draft",
    };
    expect(command).toMatchObject({
      command_name: "release",
      arguments: "v2.0 --draft",
    });
  });
});
