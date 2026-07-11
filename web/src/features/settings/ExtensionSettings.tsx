import { PackageOpen, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import type { ExtensionSourceConfig, GlobalSettings } from "../../lib/protocol";
import { Field, JsonMapField, listText, parseList, SettingsSection, Toggle } from "./SettingsFields";

type ExtensionSettingsValue = GlobalSettings["extensions"];

export function ExtensionSettings(props: { value: ExtensionSettingsValue; onChange: (value: ExtensionSettingsValue) => void }) {
  const [name, setName] = useState("");
  const sources = props.value.sources ?? {};
  const update = (sourceName: string, source: ExtensionSourceConfig) =>
    props.onChange({ ...props.value, sources: { ...sources, [sourceName]: source } });
  const remove = (sourceName: string) => {
    const next = { ...sources };
    delete next[sourceName];
    props.onChange({ ...props.value, sources: next });
  };
  const add = () => {
    const normalized = name.trim();
    if (!normalized || sources[normalized]) return;
    props.onChange({
      ...props.value,
      sources: {
        ...sources,
        [normalized]: { kind: "local", location: "", trust: "prompt", enabled: true },
      },
    });
    setName("");
  };
  return (
    <SettingsSection
      id="extensions"
      eyebrow="PI & CODEX COMPATIBILITY"
      title="Extensions and plugins"
      description="Register trusted local, Git, or registry packages without loading code into the daemon process."
    >
      <Toggle
        label="Allow unsigned packages"
        hint="Keep this disabled unless you have independently reviewed the source."
        checked={props.value.allow_unsigned}
        onChange={(allow_unsigned) => props.onChange({ ...props.value, allow_unsigned })}
      />
      <div className="inline-create">
        <label><span>Extension name</span><input value={name} onChange={(event) => setName(event.target.value)} placeholder="team-tools" /></label>
        <button className="secondary-button" onClick={add} disabled={!name.trim() || Boolean(sources[name.trim()])}><Plus size={14} /> Add source</button>
      </div>
      <div className="integration-stack">
        {Object.entries(sources).map(([sourceName, source]) => (
          <ExtensionCard key={sourceName} name={sourceName} value={source} onChange={(value) => update(sourceName, value)} onRemove={() => remove(sourceName)} />
        ))}
        {!Object.keys(sources).length && (
          <div className="integration-empty"><PackageOpen size={20} /><strong>No extension sources</strong><span>Add a Pi package or Codex-compatible plugin source.</span></div>
        )}
      </div>
    </SettingsSection>
  );
}

function ExtensionCard(props: { name: string; value: ExtensionSourceConfig; onChange: (value: ExtensionSourceConfig) => void; onRemove: () => void }) {
  const set = <K extends keyof ExtensionSourceConfig,>(key: K, value: ExtensionSourceConfig[K]) => props.onChange({ ...props.value, [key]: value });
  return (
    <article className="integration-card compact">
      <header>
        <div className="integration-icon"><PackageOpen size={16} /></div>
        <div><strong>{props.name}</strong><span>{props.value.kind} · {props.value.trust}</span></div>
        <button className="icon-button danger-icon" onClick={props.onRemove} aria-label={`Remove ${props.name}`}><Trash2 size={14} /></button>
      </header>
      <div className="settings-grid three">
        <Field label="Source type">
          <select value={props.value.kind} onChange={(event) => set("kind", event.target.value as ExtensionSourceConfig["kind"])}>
            <option value="local">Local</option><option value="git">Git</option><option value="registry">Registry</option>
          </select>
        </Field>
        <Field label="Trust">
          <select value={props.value.trust} onChange={(event) => set("trust", event.target.value as ExtensionSourceConfig["trust"])}>
            <option value="prompt">Prompt</option><option value="trusted">Trusted</option><option value="denied">Denied</option>
          </select>
        </Field>
        <div className="settings-field toggle-field"><Toggle label="Enabled" checked={props.value.enabled} onChange={(enabled) => set("enabled", enabled)} /></div>
        <Field label="Location" wide>
          <input value={props.value.location} onChange={(event) => set("location", event.target.value)} placeholder={props.value.kind === "local" ? "/absolute/path" : "owner/repository"} />
        </Field>
        <Field label="Ref" hint="Optional branch, tag or package version.">
          <input value={props.value.ref || ""} onChange={(event) => set("ref", event.target.value)} placeholder="main" />
        </Field>
        <Field label="Adapter command" wide hint="Required to execute a trusted local adapter.">
          <input value={props.value.command || ""} onChange={(event) => set("command", event.target.value)} placeholder="/absolute/path/to/adapter" />
        </Field>
        <Field label="Adapter arguments" wide hint="One argument per line.">
          <textarea rows={3} value={listText(props.value.args)} onChange={(event) => set("args", parseList(event.target.value))} />
        </Field>
        <JsonMapField label="Environment" value={props.value.env} onChange={(env) => set("env", env)} />
        <Field label="Secret environment keys" hint="Values remain redacted in this UI.">
          <textarea rows={3} value={listText(props.value.secret_env)} onChange={(event) => set("secret_env", parseList(event.target.value))} />
        </Field>
        <div className="settings-field toggle-field"><Toggle label="Inherit daemon environment" checked={Boolean(props.value.inherit_env)} onChange={(inherit_env) => set("inherit_env", inherit_env)} /></div>
      </div>
    </article>
  );
}
