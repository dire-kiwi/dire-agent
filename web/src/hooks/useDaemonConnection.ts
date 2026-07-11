import { useCallback, useEffect, useState, type MutableRefObject } from "react";
import { DaemonClient } from "../lib/daemon-client";
import type {
  ConnectionStatus,
  Conversation,
  ModelInfo,
  WireEvent,
} from "../lib/protocol";

export interface DaemonConnection {
  client: DaemonClient | null;
  status: ConnectionStatus;
  error: string;
  version: number;
  projects: Conversation[];
  chats: Conversation[];
  models: ModelInfo[];
  tools: string[];
  refreshConversations: () => Promise<void>;
  upsertConversation: (conversation: Conversation) => void;
  promoteConversation: (id: string) => void;
  removeConversation: (id: string) => void;
}

export function useDaemonConnection(
  endpoint: string,
  eventSink: MutableRefObject<(event: WireEvent) => void>,
  reconnectKey = 0,
): DaemonConnection {
  const [client, setClient] = useState<DaemonClient | null>(null);
  const [status, setStatus] = useState<ConnectionStatus>("connecting");
  const [error, setError] = useState("");
  const [version, setVersion] = useState(0);
  const [projects, setProjects] = useState<Conversation[]>([]);
  const [chats, setChats] = useState<Conversation[]>([]);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [tools, setTools] = useState<string[]>([]);

  const loadConversations = useCallback(async (active: DaemonClient) => {
    const [nextProjects, nextChats] = await Promise.all([
      active.listProjects(),
      active.listChats(),
    ]);
    setProjects(nextProjects);
    setChats(nextChats);
  }, []);

  useEffect(() => {
    let cancelled = false;
    let reconnectTimer: number | undefined;
    let attempt = 0;
    let active: DaemonClient | null = null;

    const connect = async () => {
      if (cancelled) return;
      active?.close();
      active = new DaemonClient(endpoint);
      const current = active;
      current.onEvent((event) => eventSink.current(event));
      current.onStatus((nextStatus, nextError) => {
        if (cancelled || active !== current) return;
        setStatus(nextStatus);
        setError(nextError?.message || "");
        if (nextStatus === "offline" && !current.wasManuallyClosed) {
          const delay = Math.min(10_000, 700 * 2 ** attempt) + Math.random() * 250;
          attempt += 1;
          window.clearTimeout(reconnectTimer);
          reconnectTimer = window.setTimeout(connect, delay);
        }
      });

      try {
        await current.connect();
        if (cancelled || active !== current) return;
        attempt = 0;
        const [, nextTools, nextModels] = await Promise.all([
          loadConversations(current),
          current.getAvailableTools(),
          current.getAvailableModels(),
        ]);
        if (cancelled || active !== current) return;
        setTools(nextTools);
        setModels(nextModels);
        setClient(current);
        setVersion((value) => value + 1);
      } catch (cause) {
        if (cancelled || active !== current) return;
        setError(cause instanceof Error ? cause.message : "Could not connect to daemon");
        if (current.isOpen) {
          current.close();
          window.clearTimeout(reconnectTimer);
          reconnectTimer = window.setTimeout(connect, 1_000);
        }
      }
    };

    void connect();
    return () => {
      cancelled = true;
      window.clearTimeout(reconnectTimer);
      active?.close();
      setClient(null);
    };
  }, [endpoint, eventSink, loadConversations, reconnectKey]);

  const refreshConversations = useCallback(async () => {
    if (client?.isOpen) await loadConversations(client);
  }, [client, loadConversations]);

  const upsertConversation = useCallback((conversation: Conversation) => {
    const setter = conversation.kind === "chat" || conversation.id.startsWith("chat_")
      ? setChats
      : setProjects;
    setter((current) => {
      const index = current.findIndex((item) => item.id === conversation.id);
      if (index < 0) return [conversation, ...current];
      return current.map((item, itemIndex) => itemIndex === index ? conversation : item);
    });
  }, []);

  const promoteConversation = useCallback((id: string) => {
    const promote = (current: Conversation[]) => {
      const index = current.findIndex((item) => item.id === id);
      if (index <= 0) return current;
      return [current[index], ...current.slice(0, index), ...current.slice(index + 1)];
    };
    setProjects(promote);
    setChats(promote);
  }, []);

  const removeConversation = useCallback((id: string) => {
    setProjects((current) => current.filter((item) => item.id !== id));
    setChats((current) => current.filter((item) => item.id !== id));
  }, []);

  return {
    client,
    status,
    error,
    version,
    projects,
    chats,
    models,
    tools,
    refreshConversations,
    upsertConversation,
    promoteConversation,
    removeConversation,
  };
}
