import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { unsupported, type DaemonClient } from "../lib/daemon-client";
import {
  conversationScope,
  wireConversationID,
  type Conversation,
  type SpawnAgentOptions,
  type StoredMessage,
  type SubagentDetail,
  type SubagentInfo,
  type SubagentMessage,
  type WireEvent,
} from "../lib/protocol";

interface SubagentOptions {
  client: DaemonClient | null;
  resource: Conversation | null;
  connectionVersion: number;
  onNotice: (message: string) => void;
}

export interface SubagentController {
  agents: SubagentInfo[];
  selected: SubagentInfo | null;
  messages: SubagentMessage[];
  supported: boolean;
  loading: boolean;
  waiting: boolean;
  handleEvent: (event: WireEvent) => void;
  select: (id: string) => Promise<void>;
  spawn: (options: SpawnAgentOptions) => Promise<boolean>;
  send: (message: string) => Promise<boolean>;
  interrupt: (id?: string) => Promise<boolean>;
  wait: (id?: string) => Promise<boolean>;
  remove: (id?: string) => Promise<boolean>;
  refresh: () => Promise<void>;
}

export function useSubagents(options: SubagentOptions): SubagentController {
  const { client, resource, connectionVersion, onNotice } = options;
  const [agents, setAgents] = useState<SubagentInfo[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [messagesByAgent, setMessages] = useState<Record<string, SubagentMessage[]>>({});
  const [supported, setSupported] = useState(true);
  const [loading, setLoading] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const resourceRef = useRef(resource);
  const agentsRef = useRef(agents);
  resourceRef.current = resource;
  agentsRef.current = agents;

  const refresh = useCallback(async () => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active) return;
    setLoading(true);
    try {
      const next = (await client.listAgents(active)).map(normalizeAgent).filter((item) => item.id);
      setAgents((current) => next.map((agent) => {
        const previous = current.find((item) => item.id === agent.id);
        return previous?.task && !agent.task ? { ...agent, task: previous.task } : agent;
      }));
      setSupported(true);
      setSelectedID((current) => current && next.some((item) => item.id === current) ? current : "");
    } catch (error) {
      if (unsupported(error)) {
        setSupported(false);
        setAgents([]);
      } else {
        onNotice(error instanceof Error ? error.message : "Could not load subagents");
      }
    } finally {
      setLoading(false);
    }
  }, [client, onNotice]);

  useEffect(() => {
    setAgents([]);
    setSelectedID("");
    setMessages({});
    if (resource && client?.isOpen) void refresh();
  }, [client, connectionVersion, resource?.id, refresh]);

  useEffect(() => {
    if (!resource || !supported || !agents.some((agent) => agent.status === "running")) return;
    const timer = window.setInterval(() => void refresh(), 2_000);
    return () => window.clearInterval(timer);
  }, [agents, refresh, resource, supported]);

  const upsert = useCallback((agent: SubagentInfo) => {
    if (!agent.id) return;
    setAgents((current) => {
      const previous = current.find((item) => item.id === agent.id);
      const merged = previous?.task && !agent.task ? { ...agent, task: previous.task } : agent;
      return [
        ...current.filter((item) => item.id !== agent.id),
        merged,
      ].sort((left, right) => left.depth - right.depth || left.agent_name.localeCompare(right.agent_name));
    });
  }, []);

  const handleEvent = useCallback((event: WireEvent) => {
    const agentEvents = [
      "agent_created", "agent_spawned", "agent_message", "agent_message_sent",
      "agent_status", "agent_completed", "agent_settled",
    ];
    const active = resourceRef.current;
    const conversationID = wireConversationID(event);
    const scopedAgent = agentsRef.current.find((item) => item.id === conversationID);
    const genericChildEvent = Boolean(
      scopedAgent && scopedAgent.id !== active?.id &&
      ["agent_start", "message_end", "tool_execution_end"].includes(event.type),
    );
    if (!agentEvents.includes(event.type) && !genericChildEvent) return;
    if (!active || (conversationID && conversationID !== active.id && !scopedAgent)) return;
    setSupported(true);
    const data = event.data ?? {};
    if (genericChildEvent && scopedAgent) {
      if (event.type === "agent_start") {
        upsert({ ...scopedAgent, status: "running" });
      } else {
        const content = event.type === "message_end"
          ? stringValue(data.text)
          : stringValue(data.output);
        if (content) {
          setMessages((current) => appendAgentMessage(current, scopedAgent.id, {
            id: stringValue(data.message_id) || stringValue(data.tool_call_id) || undefined,
            agent_id: scopedAgent.id,
            role: event.type === "message_end" ? "agent" : "system",
            content,
            created_at: event.timestamp,
          }));
        }
      }
      return;
    }
    const isMessage = event.type === "agent_message" || event.type === "agent_message_sent";
    const agentSource = data.agent && typeof data.agent === "object" ? data.agent : (isMessage ? {} : data);
    const agent = normalizeAgent(agentSource);

    if (isMessage) {
      const fromID = stringValue(data.from_agent_id) || stringValue(data.from_id);
      const toID = stringValue(data.to_id);
      const agentID = stringValue(data.agent_id) || (fromID && fromID !== active.id ? fromID : toID);
      const content = stringValue(data.content) || stringValue(data.message);
      if (agentID && content) {
        setMessages((current) => appendAgentMessage(current, agentID, {
          id: stringValue(data.id) || undefined,
          agent_id: agentID,
          from_agent_id: fromID || undefined,
          from_id: fromID || undefined,
          to_id: toID || undefined,
          role: stringValue(data.role) || (fromID === active.id ? "user" : "agent"),
          content,
          created_at: event.timestamp,
        }));
      }
    }
    if (event.type === "agent_settled" && scopedAgent) {
      upsert({ ...scopedAgent, status: "idle" });
      void refresh();
      return;
    }
    if (agent.id) {
      const status = event.type === "agent_completed"
        ? stringValue(data.status) || agent.status || "completed"
        : agent.status;
      const completion = stringValue(data.result) || stringValue(data.error);
      if (event.type === "agent_completed" && completion) {
        setMessages((current) => appendAgentMessage(current, agent.id, {
          id: `completion-${agent.id}-${event.timestamp}`,
          agent_id: agent.id,
          role: stringValue(data.error) ? "system" : "agent",
          content: completion,
          created_at: event.timestamp,
        }));
      }
      upsert({ ...agent, status, error: stringValue(data.error) || agent.error });
    } else if (!isMessage && event.type !== "agent_settled") {
      void refresh();
    }
  }, [refresh, upsert]);

  const select = useCallback(async (id: string) => {
    setSelectedID(id);
    const active = resourceRef.current;
    if (!client?.isOpen || !active) return;
    try {
      const [response, transcript] = await Promise.all([
        client.getAgent(active, id),
        client.getAgentMessages(id),
      ]);
      const detail = response as SubagentDetail;
      const agent = detail.agent ? normalizeAgent(detail.agent) : normalizeAgent(response);
      upsert(agent);
      if (detail.messages?.length) {
        setMessages((current) => ({
          ...current,
          [id]: (detail.messages ?? []).map((message) => normalizeMessage(message, active.id, id)),
        }));
      } else if (transcript.length) {
        setMessages((current) => ({
          ...current,
          [id]: hydrateAgentTranscript(transcript, active.id, id),
        }));
      }
      if (detail.children) detail.children.forEach((child) => upsert(normalizeAgent(child)));
    } catch (error) {
      if (!unsupported(error)) onNotice(error instanceof Error ? error.message : "Could not load agent");
    }
  }, [client, onNotice, upsert]);

  const spawn = useCallback(async (spawnOptions: SpawnAgentOptions): Promise<boolean> => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active) return false;
    try {
      const agent = normalizeAgent(await client.spawnAgent(active, spawnOptions));
      if (!agent.task) agent.task = spawnOptions.task;
      upsert(agent);
      setSelectedID(agent.id);
      setSupported(true);
      return true;
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Could not spawn subagent");
      return false;
    }
  }, [client, onNotice, upsert]);

  const send = useCallback(async (message: string): Promise<boolean> => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active || !selectedID || !message.trim()) return false;
    try {
      const response = await client.sendAgentMessage(active, selectedID, message.trim());
      setMessages((current) => appendAgentMessage(
        current,
        selectedID,
        normalizeMessage(response, active.id, selectedID),
      ));
      return true;
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Could not send agent message");
      return false;
    }
  }, [client, onNotice, selectedID]);

  const interrupt = useCallback(async (id = selectedID): Promise<boolean> => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active || !id) return false;
    try {
      await client.interruptAgent(active, id);
      setAgents((current) => current.map((item) => item.id === id ? { ...item, status: "interrupted" } : item));
      return true;
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Could not interrupt agent");
      return false;
    }
  }, [client, onNotice, selectedID]);

  const remove = useCallback(async (id = selectedID): Promise<boolean> => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active || !id) return false;
    try {
      await client.deleteAgent(active, id);
      setAgents((current) => current.filter((item) => item.id !== id));
      setMessages((current) => {
        const next = { ...current };
        delete next[id];
        return next;
      });
      setSelectedID("");
      return true;
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Could not delete agent");
      return false;
    }
  }, [client, onNotice, selectedID]);

  const wait = useCallback(async (id = selectedID): Promise<boolean> => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active) return false;
    const selectedAgent = id ? agentsRef.current.find((item) => item.id === id) : undefined;
    const agentIDs = selectedAgent && selectedAgent.depth > 0 ? [selectedAgent.id] : [];
    setWaiting(true);
    try {
      const result = await client.waitAgents(active, agentIDs, 10_000);
      for (const agent of result.agents ?? []) upsert(normalizeAgent(agent));
      for (const message of result.messages ?? []) {
        const agentID = message.agent_id || message.from_agent_id || message.from_id || message.to_id;
        if (!agentID) continue;
        setMessages((current) => appendAgentMessage(current, agentID, normalizeMessage(message, active.id, agentID)));
      }
      onNotice(result.timed_out ? "Agent wait timed out" : "Agent wait completed");
      await refresh();
      return !result.timed_out;
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Could not wait for agents");
      return false;
    } finally {
      setWaiting(false);
    }
  }, [client, onNotice, refresh, selectedID, upsert]);

  const selected = useMemo(() => agents.find((item) => item.id === selectedID) ?? null, [agents, selectedID]);
  return { agents, selected, messages: selected ? messagesByAgent[selected.id] ?? [] : [], supported, loading, waiting, handleEvent, select, spawn, send, interrupt, wait, remove, refresh };
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function normalizeAgent(value: unknown): SubagentInfo {
  const source = value && typeof value === "object" ? value as Partial<SubagentInfo> & Record<string, unknown> : {};
  return {
    id: stringValue(source.id) || stringValue(source.agent_id),
    conversation_id: stringValue(source.conversation_id) || undefined,
    parent_id: stringValue(source.parent_id) || undefined,
    root_id: stringValue(source.root_id) || undefined,
    agent_name: stringValue(source.agent_name) || stringValue(source.name) || "Agent",
    agent_role: stringValue(source.agent_role) || stringValue(source.role) || undefined,
    profile: stringValue(source.profile) || stringValue(source.agent_profile) || undefined,
    task: stringValue(source.task) || undefined,
    model: stringValue(source.model) || undefined,
    depth: typeof source.depth === "number" ? source.depth : 1,
    status: stringValue(source.status) || "idle",
    error: stringValue(source.error) || undefined,
    created_at: stringValue(source.created_at) || undefined,
    updated_at: stringValue(source.updated_at) || undefined,
  };
}

function normalizeMessage(value: unknown, rootID: string, agentID: string): SubagentMessage {
  const source = value && typeof value === "object" ? value as Partial<SubagentMessage> : {};
  const fromID = source.from_agent_id || source.from_id;
  return {
    id: source.id,
    agent_id: source.agent_id || agentID,
    from_agent_id: fromID,
    from_id: source.from_id,
    to_id: source.to_id,
    role: source.role || (fromID === rootID ? "user" : "agent"),
    content: source.content || "",
    created_at: source.created_at,
  };
}

function appendAgentMessage(
  current: Record<string, SubagentMessage[]>,
  agentID: string,
  message: SubagentMessage,
): Record<string, SubagentMessage[]> {
  const messages = current[agentID] ?? [];
  if (message.id && messages.some((item) => item.id === message.id)) return current;
  return { ...current, [agentID]: [...messages, message] };
}

function hydrateAgentTranscript(
  messages: StoredMessage[],
  rootID: string,
  agentID: string,
): SubagentMessage[] {
  return messages.filter((message) => Boolean(message.content?.trim())).map((message) => {
    const fromID = stringValue(message.data?.from_id);
    const role = message.role === "user" || fromID === rootID
      ? "user"
      : message.role === "assistant" || message.role === "agent"
        ? "agent"
        : "system";
    return {
      id: `stored-${message.sequence}`,
      agent_id: agentID,
      from_id: fromID || undefined,
      to_id: stringValue(message.data?.to_id) || undefined,
      role,
      content: message.content || "",
      created_at: message.created_at,
    };
  });
}
