import type { StoredMessage, WireEvent } from "./protocol";

export type MessageRole =
  | "user"
  | "assistant"
  | "reasoning"
  | "tool"
  | "system"
  | "error";

export interface ChatMessage {
  id: string;
  role: MessageRole;
  content: string;
  label?: string;
  createdAt?: string;
  pending?: boolean;
  arguments?: unknown;
  attachments?: ChatImage[];
}

export interface ChatImage {
  name: string;
  mimeType: string;
  url: string;
}

export interface ActiveTool {
  id: string;
  name: string;
  arguments?: unknown;
}

export interface ConversationState {
  messages: ChatMessage[];
  stream: { id: string; content: string } | null;
  reasoning: { id: string; content: string } | null;
  activeTools: ActiveTool[];
  running: boolean;
  steeringQueued: number;
  followUpsQueued: number;
  lastSequence: number;
  hasSequenceGap: boolean;
}

export const emptyConversation: ConversationState = {
  messages: [],
  stream: null,
  reasoning: null,
  activeTools: [],
  running: false,
  steeringQueued: 0,
  followUpsQueued: 0,
  lastSequence: 0,
  hasSequenceGap: false,
};

function stringField(
  data: Record<string, unknown> | undefined,
  field: string,
): string {
  const value = data?.[field];
  return typeof value === "string" ? value : "";
}

function numberField(
  data: Record<string, unknown> | undefined,
  field: string,
): number {
  const value = data?.[field];
  return typeof value === "number" ? value : 0;
}

export function hydrateMessages(
  messages: StoredMessage[],
  attachmentURL?: (file: string) => string,
): ChatMessage[] {
  return (messages ?? [])
    .filter((message) => Boolean(message.content?.trim()) || storedAttachments(message.data).length > 0)
    .map((message) => {
      const rawRole = message.role || message.kind;
      const role: MessageRole =
        rawRole === "user" ||
        rawRole === "assistant" ||
        rawRole === "reasoning" ||
        rawRole === "tool" ||
        rawRole === "error"
          ? rawRole
          : "system";
      return {
        id: `stored-${message.sequence}`,
        role,
        content: role === "reasoning" ? cleanReasoningText(message.content || "") : message.content || "",
        label:
          role === "tool" ? stringField(message.data, "tool_name") : undefined,
        arguments: role === "tool" ? message.data?.arguments : undefined,
        createdAt: message.created_at,
        attachments: storedAttachments(message.data).map((attachment) => ({
          name: attachment.name || "Pasted image",
          mimeType: attachment.mime_type,
          url: attachment.file && attachmentURL ? attachmentURL(attachment.file) : "",
        })).filter((attachment) => Boolean(attachment.url)),
      };
    });
}

function storedAttachments(data: Record<string, unknown> | undefined): Array<{
  name?: string;
  mime_type: string;
  file?: string;
}> {
  if (!Array.isArray(data?.attachments)) return [];
  return data.attachments.filter((value): value is { name?: string; mime_type: string; file?: string } =>
    Boolean(value && typeof value === "object" && typeof (value as Record<string, unknown>).mime_type === "string"));
}

export function reduceEvent(
  current: ConversationState,
  event: WireEvent,
): ConversationState {
  const sequence = event.sequence || 0;
  if (sequence > 0 && sequence <= current.lastSequence) return current;

  let state: ConversationState = {
    ...current,
    lastSequence: Math.max(current.lastSequence, sequence),
    hasSequenceGap:
      current.hasSequenceGap ||
      (current.lastSequence > 0 && sequence > current.lastSequence + 1),
  };
  const data = event.data;

  switch (event.type) {
    case "agent_start":
      return { ...state, running: true };
    case "message_start":
      return {
        ...state,
        stream: { id: stringField(data, "message_id") || `live-${sequence}`, content: "" },
      };
    case "message_update": {
      const id = stringField(data, "message_id") || `live-${sequence}`;
      const previous = state.stream?.id === id ? state.stream.content : "";
      return {
        ...state,
        stream: { id, content: previous + stringField(data, "delta") },
      };
    }
    case "message_end": {
      const text = stringField(data, "text") || state.stream?.content || "";
      if (!text.trim()) return { ...state, stream: null };
      return {
        ...state,
        stream: null,
        messages: [
          ...state.messages,
          {
            id: stringField(data, "message_id") || `assistant-${sequence}`,
            role: "assistant",
            content: text,
            createdAt: event.timestamp,
          },
        ],
      };
    }
    case "reasoning_start":
      return {
        ...state,
        reasoning: {
          id: stringField(data, "message_id") || `reasoning-${sequence}`,
          content: "",
        },
      };
    case "reasoning_update": {
      const id = stringField(data, "message_id") || `reasoning-${sequence}`;
      const previous = state.reasoning?.id === id ? state.reasoning.content : "";
      return {
        ...state,
        reasoning: { id, content: previous + stringField(data, "delta") },
      };
    }
    case "reasoning_end": {
      const text = cleanReasoningText(stringField(data, "text") || state.reasoning?.content || "");
      if (!text.trim()) return { ...state, reasoning: null };
      return {
        ...state,
        reasoning: null,
        messages: [
          ...state.messages,
          {
            id: stringField(data, "message_id") || `reasoning-${sequence}`,
            role: "reasoning",
            content: text,
            createdAt: event.timestamp,
          },
        ],
      };
    }
    case "tool_execution_start": {
      const id = stringField(data, "tool_call_id") || `tool-${sequence}`;
      return {
        ...state,
        activeTools: [
          ...state.activeTools.filter((tool) => tool.id !== id),
          {
            id,
            name: stringField(data, "tool_name") || "tool",
            arguments: data?.arguments,
          },
        ],
      };
    }
    case "tool_execution_end": {
      const id = stringField(data, "tool_call_id") || `tool-${sequence}`;
      const failed = data?.is_error === true;
      return {
        ...state,
        activeTools: state.activeTools.filter((tool) => tool.id !== id),
        messages: [
          ...state.messages,
          {
            id: `tool-result-${id}-${sequence}`,
            role: failed ? "error" : "tool",
            label: stringField(data, "tool_name") || "tool",
            arguments: data?.arguments,
            content: stringField(data, "output") || "Completed without output.",
            createdAt: event.timestamp,
          },
        ],
      };
    }
    case "queue_update":
      return {
        ...state,
        steeringQueued: numberField(data, "steering"),
        followUpsQueued: numberField(data, "follow_ups"),
      };
    case "agent_error":
    case "persistence_error":
      return {
        ...state,
        messages: [
          ...state.messages,
          {
            id: `error-${sequence}`,
            role: "error",
            content: stringField(data, "error") || "The agent failed.",
            createdAt: event.timestamp,
          },
        ],
      };
    case "agent_settled":
      return {
        ...state,
        running: false,
        reasoning: null,
        activeTools: [],
        steeringQueued: 0,
        followUpsQueued: 0,
      };
    default:
      return state;
  }
}

export function cleanReasoningText(value: string): string {
  return value.replace(/<!--[\s\S]*?-->/g, "").trim();
}

export interface SlashCommand {
  name: string;
  description: string;
  acceptsArgument?: boolean;
}

export const slashCommands: SlashCommand[] = [
  { name: "steer", description: "Guide the active run", acceptsArgument: true },
  { name: "follow-up", description: "Queue the next turn", acceptsArgument: true },
  { name: "abort", description: "Cancel the active run" },
  { name: "model", description: "Show or change the model", acceptsArgument: true },
  { name: "thinking", description: "Show or change reasoning", acceptsArgument: true },
  { name: "name", description: "Show or rename this conversation", acceptsArgument: true },
  { name: "folders", description: "Show main and additional sandbox folders" },
  { name: "folder-add", description: "Include an absolute sandbox folder", acceptsArgument: true },
  { name: "folder-remove", description: "Remove an included sandbox folder", acceptsArgument: true },
  { name: "status", description: "Show state, queues, and usage" },
  { name: "clear", description: "Clear the local transcript" },
  { name: "help", description: "Show command help" },
  { name: "quit", description: "Disconnect this browser client" },
];

export function slashCommandSuggestions(input: string): SlashCommand[] {
  const value = input.trimStart();
  if (!value.startsWith("/") || value.includes("\n") || /\s/.test(value.slice(1))) return [];
  const query = value.slice(1).toLowerCase();
  const matches = slashCommands.filter((command) => command.name.startsWith(query));
  if (matches.length === 1 && matches[0].name === query) return [];
  return matches;
}

export function completeSlashCommand(command: SlashCommand): string {
  return `/${command.name}${command.acceptsArgument ? " " : ""}`;
}

export type ComposerAction =
  | { kind: "prompt"; value: string }
  | { kind: "steer"; value: string }
  | { kind: "follow-up"; value: string }
  | { kind: "abort" }
  | { kind: "model"; value: string }
  | { kind: "thinking"; value: string }
  | { kind: "name"; value: string }
  | { kind: "folders" }
  | { kind: "folder-add"; value: string }
  | { kind: "folder-remove"; value: string }
  | { kind: "status" }
  | { kind: "clear" }
  | { kind: "help" }
  | { kind: "quit" };

export function parseComposerInput(input: string): ComposerAction {
  const trimmed = input.trim();
  if (!trimmed.startsWith("/")) return { kind: "prompt", value: trimmed };

  const [command, ...rest] = trimmed.split(/\s+/);
  const value = rest.join(" ").trim();
  const name = command.slice(1).toLowerCase();
  const required = (kind: "steer" | "follow-up"): ComposerAction => {
    if (!value) throw new Error(`/${kind} requires text`);
    return { kind, value };
  };

  switch (name) {
    case "steer":
      return required("steer");
    case "follow-up":
    case "followup":
    case "follow_up":
    case "queue":
      return required("follow-up");
    case "abort":
    case "stop":
      return { kind: "abort" };
    case "model":
      return { kind: "model", value };
    case "thinking":
    case "think":
      return { kind: "thinking", value: value.toLowerCase() };
    case "name":
    case "rename":
      return { kind: "name", value };
    case "folders":
      return { kind: "folders" };
    case "folder-add":
      if (!value) throw new Error("/folder-add requires an absolute path");
      return { kind: "folder-add", value };
    case "folder-remove":
      if (!value) throw new Error("/folder-remove requires a canonical path");
      return { kind: "folder-remove", value };
    case "status":
    case "thread":
      return { kind: "status" };
    case "clear":
      return { kind: "clear" };
    case "help":
    case "?":
      return { kind: "help" };
    case "quit":
    case "exit":
    case "q":
      return { kind: "quit" };
    default:
      throw new Error(`Unknown command /${name}. Type /help.`);
  }
}
