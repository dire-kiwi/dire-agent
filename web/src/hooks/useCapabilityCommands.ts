import { useCallback, useEffect, useRef, useState } from "react";
import { unsupported, type DaemonClient } from "../lib/daemon-client";
import {
  wireConversationID,
  type CapabilityCommandInfo,
  type CapabilityCommandResult,
  type Conversation,
  type WireEvent,
} from "../lib/protocol";

interface Options {
  client: DaemonClient | null;
  resource: Conversation | null;
  connectionVersion: number;
}

export interface CapabilityCommandController {
  commands: CapabilityCommandInfo[];
  loading: boolean;
  executing: boolean;
  supported: boolean;
  result: CapabilityCommandResult | null;
  error: string;
  refresh: () => Promise<void>;
  execute: (name: string, commandArguments: string) => Promise<boolean>;
  clearResult: () => void;
  handleEvent: (event: WireEvent) => void;
}

export function useCapabilityCommands(options: Options): CapabilityCommandController {
  const { client, resource, connectionVersion } = options;
  const [commands, setCommands] = useState<CapabilityCommandInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [supported, setSupported] = useState(true);
  const [result, setResult] = useState<CapabilityCommandResult | null>(null);
  const [error, setError] = useState("");
  const resourceRef = useRef(resource);
  resourceRef.current = resource;

  const refresh = useCallback(async () => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active) return;
    setLoading(true);
    setError("");
    try {
      setCommands(await client.listCapabilityCommands(active));
      setSupported(true);
    } catch (cause) {
      if (unsupported(cause)) {
        setCommands([]);
        setSupported(false);
      } else {
        setError(cause instanceof Error ? cause.message : "Could not list extension commands");
      }
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    setCommands([]);
    setResult(null);
    setError("");
    if (resource && client?.isOpen) void refresh();
  }, [client, connectionVersion, refresh, resource?.id]);

  const execute = useCallback(async (name: string, commandArguments: string) => {
    const active = resourceRef.current;
    if (!client?.isOpen || !active || !name) return false;
    setExecuting(true);
    setError("");
    setResult(null);
    try {
      const next = await client.executeCapabilityCommand(active, name, commandArguments);
      setResult(next);
      if (next.is_error) setError(next.output || "Extension command failed");
      return !next.is_error;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Extension command failed");
      return false;
    } finally {
      setExecuting(false);
    }
  }, [client]);

  const clearResult = useCallback(() => {
    setResult(null);
    setError("");
  }, []);

  const handleEvent = useCallback((event: WireEvent) => {
    const active = resourceRef.current;
    if (!active || wireConversationID(event) !== active.id) return;
    if (event.type === "capabilities_updated" || event.type === "extensions_changed") {
      void refresh();
    }
  }, [refresh]);

  return {
    commands,
    loading,
    executing,
    supported,
    result,
    error,
    refresh,
    execute,
    clearResult,
    handleEvent,
  };
}
