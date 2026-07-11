import { useCallback, useEffect, useMemo, useState } from "react";
import type { DaemonClient } from "../lib/daemon-client";
import type { DaemonConfig, GlobalSettings } from "../lib/protocol";

interface SettingsOptions {
  client: DaemonClient | null;
  active: boolean;
  connectionVersion: number;
}

export interface SettingsController {
  config: DaemonConfig | null;
  draft: DaemonConfig | null;
  loading: boolean;
  saving: boolean;
  dirty: boolean;
  error: string;
  conflict: boolean;
  setGlobal: (updater: (settings: GlobalSettings) => GlobalSettings) => void;
  reload: () => Promise<void>;
  save: () => Promise<boolean>;
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export function useSettings(options: SettingsOptions): SettingsController {
  const { client, active, connectionVersion } = options;
  const [config, setConfig] = useState<DaemonConfig | null>(null);
  const [draft, setDraft] = useState<DaemonConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);

  const reload = useCallback(async () => {
    if (!client?.isOpen) return;
    setLoading(true);
    setError("");
    setConflict(false);
    try {
      const next = await client.getConfig();
      setConfig(next);
      setDraft(clone(next));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load configuration");
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    if (active && client?.isOpen) void reload();
  }, [active, client, connectionVersion, reload]);

  const dirty = useMemo(
    () => Boolean(config && draft && JSON.stringify(config) !== JSON.stringify(draft)),
    [config, draft],
  );

  const setGlobal = useCallback((updater: (settings: GlobalSettings) => GlobalSettings) => {
    setDraft((current) => current ? { ...current, global: updater(current.global) } : current);
    setError("");
    setConflict(false);
  }, []);

  const save = useCallback(async (): Promise<boolean> => {
    if (!client?.isOpen || !config || !draft) return false;
    setSaving(true);
    setError("");
    setConflict(false);
    try {
      await client.validateConfig(draft);
      const updated = await client.updateConfig(draft, config.revision);
      setConfig(updated);
      setDraft(clone(updated));
      return true;
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not save configuration";
      setError(message);
      setConflict(/revision conflict|expected \d+, actual \d+/i.test(message));
      return false;
    } finally {
      setSaving(false);
    }
  }, [client, config, draft]);

  return { config, draft, loading, saving, dirty, error, conflict, setGlobal, reload, save };
}
