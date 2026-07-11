import { ArrowDown, ArrowUp, MonitorUp, Plus, SquareTerminal, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import type { ProjectLauncher } from "../../lib/configuration";
import { Field, listText, SettingsSection } from "./SettingsFields";

interface WorkspaceLaunchersSettingsProps {
  value: ProjectLauncher[];
  onChange: (value: ProjectLauncher[]) => void;
}

export function WorkspaceLaunchersSettings(props: WorkspaceLaunchersSettingsProps) {
  const update = (index: number, launcher: ProjectLauncher) => {
    props.onChange(props.value.map((current, currentIndex) => currentIndex === index ? launcher : current));
  };
  const remove = (index: number) => {
    props.onChange(props.value.filter((_, currentIndex) => currentIndex !== index));
  };
  const move = (index: number, offset: -1 | 1) => {
    const destination = index + offset;
    if (destination < 0 || destination >= props.value.length) return;
    const next = [...props.value];
    [next[index], next[destination]] = [next[destination], next[index]];
    props.onChange(next);
  };
  const add = (kind: ProjectLauncher["kind"]) => {
    const baseID = kind === "terminal" ? "terminal" : "desktop";
    const id = availableID(baseID, props.value);
    props.onChange([
      ...props.value,
      {
        id,
        label: kind === "terminal" ? "Terminal" : "Desktop app",
        kind,
        command: "",
        args: [],
        shortcut: "",
      },
    ]);
  };

  return (
    <SettingsSection
      id="workspace-launchers"
      eyebrow="PROJECT WORKSPACE"
      title="Workspace tabs"
      description="Choose the ordered terminal, TUI, and desktop launchers shown beside each project conversation."
    >
      <div className="inline-create">
        <button className="secondary-button" type="button" onClick={() => add("terminal")}>
          <SquareTerminal size={14} /> Add terminal
        </button>
        <button className="secondary-button" type="button" onClick={() => add("desktop")}>
          <MonitorUp size={14} /> Add desktop
        </button>
      </div>

      <div className="integration-stack">
        {props.value.map((launcher, index) => (
          <LauncherCard
            key={index}
            value={launcher}
            index={index}
            count={props.value.length}
            onChange={(next) => update(index, next)}
            onMove={(offset) => move(index, offset)}
            onRemove={() => remove(index)}
          />
        ))}
        {!props.value.length && (
          <div className="integration-empty">
            <Plus size={20} />
            <strong>No workspace tabs configured</strong>
            <span>Add a terminal or desktop launcher for folder-scoped projects.</span>
          </div>
        )}
      </div>

      <p className="secret-note">
        Shortcuts use canonical names such as <code>mod+backquote</code> and <code>mod+shift+g</code>.
        The <code>mod</code> key means Command on macOS and Control elsewhere.
      </p>
    </SettingsSection>
  );
}

function LauncherCard(props: {
  value: ProjectLauncher;
  index: number;
  count: number;
  onChange: (value: ProjectLauncher) => void;
  onMove: (offset: -1 | 1) => void;
  onRemove: () => void;
}) {
  const { value } = props;
  const name = value.label.trim() || value.id.trim() || `Launcher ${props.index + 1}`;
  const set = <K extends keyof ProjectLauncher,>(key: K, next: ProjectLauncher[K]) => {
    props.onChange({ ...value, [key]: next });
  };
  const Icon = value.kind === "desktop" ? MonitorUp : SquareTerminal;

  return (
    <article className="integration-card compact" aria-label={`${name} launcher`}>
      <header>
        <div className="integration-icon"><Icon size={16} /></div>
        <div><strong>{name}</strong><span>{value.kind} · position {props.index + 1}</span></div>
        <button
          className="icon-button"
          type="button"
          onClick={() => props.onMove(-1)}
          disabled={props.index === 0}
          aria-label={`Move ${name} up`}
        >
          <ArrowUp size={14} />
        </button>
        <button
          className="icon-button"
          type="button"
          onClick={() => props.onMove(1)}
          disabled={props.index === props.count - 1}
          aria-label={`Move ${name} down`}
        >
          <ArrowDown size={14} />
        </button>
        <button className="icon-button danger-icon" type="button" onClick={props.onRemove} aria-label={`Remove ${name}`}>
          <Trash2 size={14} />
        </button>
      </header>

      <div className="settings-grid three">
        <Field label="Label">
          <input value={value.label} onChange={(event) => set("label", event.target.value)} />
        </Field>
        <Field label="ID" hint="Stable identifier used by shortcuts and running tabs.">
          <input value={value.id} onChange={(event) => set("id", event.target.value)} spellCheck={false} />
        </Field>
        <Field label="Kind">
          <select value={value.kind} onChange={(event) => set("kind", event.target.value as ProjectLauncher["kind"])}>
            <option value="terminal">Terminal / TUI</option>
            <option value="desktop">Desktop application</option>
          </select>
        </Field>
        <Field
          label="Command"
          wide
          hint={value.kind === "terminal"
            ? "Leave blank to open the daemon user's login shell. A configured command executes directly without shell parsing."
            : "Required for desktop launchers and executed directly on the daemon host without shell parsing."}
        >
          <input
            value={value.command || ""}
            onChange={(event) => set("command", event.target.value)}
            placeholder={value.kind === "terminal" ? "Login shell" : "open"}
            spellCheck={false}
          />
        </Field>
        <Field label="Shortcut" hint="Optional canonical shortcut; it must not conflict with another tab.">
          <input
            value={value.shortcut || ""}
            onChange={(event) => set("shortcut", event.target.value)}
            placeholder="mod+shift+g"
            spellCheck={false}
          />
        </Field>
        <ArgumentsField value={value.args} onChange={(args) => set("args", args)} />
      </div>
    </article>
  );
}

function ArgumentsField(props: { value?: string[]; onChange: (value: string[]) => void }) {
  const serialized = listText(props.value);
  const [draft, setDraft] = useState(serialized);
  useEffect(() => setDraft(serialized), [serialized]);
  const commit = () => props.onChange(draft.split(/\r?\n/).map((argument) => argument.trim()).filter(Boolean));
  return (
    <Field label="Arguments" wide hint="One direct argument per line; values are not interpreted by a shell.">
      <textarea
        rows={3}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        spellCheck={false}
      />
    </Field>
  );
}

function availableID(base: string, launchers: ProjectLauncher[]): string {
  const used = new Set(launchers.map((launcher) => launcher.id));
  if (!used.has(base)) return base;
  for (let suffix = 2; ; suffix += 1) {
    const candidate = `${base}-${suffix}`;
    if (!used.has(candidate)) return candidate;
  }
}
