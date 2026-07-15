export type SubagentStatus =
  | "running"
  | "idle"
  | "error"
  | "completed"
  | "interrupted"
  | string;

export interface SubagentInfo {
  id: string;
  conversation_id?: string;
  parent_id?: string;
  root_id?: string;
  agent_name: string;
  agent_role?: string;
  profile?: string;
  task?: string;
  model?: string;
  depth: number;
  status: SubagentStatus;
  error?: string;
  created_at?: string;
  updated_at?: string;
}

/** Raw agentteam shape returned by the daemon WebSocket API. */
export interface WireSubagentInfo {
  id: string;
  parent_id: string;
  root_id: string;
  name: string;
  role?: string;
  profile?: string;
  depth: number;
  status: SubagentStatus;
  model?: string;
  created_at?: string;
  updated_at?: string;
}

export interface SpawnAgentOptions {
  parent_id?: string;
  agent_name: string;
  agent_role?: string;
  task: string;
  mode?: "direct" | "model-router";
  model?: string;
  profile?: string;
  level?: string;
  tools?: string[];
}

export interface SubagentMessage {
  id?: string;
  agent_id: string;
  from_agent_id?: string;
  from_id?: string;
  to_id?: string;
  role?: "user" | "agent" | "system" | string;
  content: string;
  created_at?: string;
}

export interface SubagentDetail {
  agent: SubagentInfo | WireSubagentInfo;
  messages?: SubagentMessage[];
  children?: SubagentInfo[];
}

export interface SubagentWaitResult {
  agents: WireSubagentInfo[];
  messages?: SubagentMessage[];
  timed_out?: boolean;
}
