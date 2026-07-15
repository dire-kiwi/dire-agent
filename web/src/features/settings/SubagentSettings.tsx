import { Bot, Plus, Trash2, Users } from "lucide-react";
import { useEffect, useState } from "react";
import { configurationThinkingLevels } from "../../lib/display";
import type { AgentProfile, SubagentSettings as SubagentSettingsValue } from "../../lib/protocol";
import { Field, listText, parseList, SettingsSection, Toggle } from "./SettingsFields";

export function SubagentSettings(props: { value: SubagentSettingsValue; onChange: (value: SubagentSettingsValue) => void }) {
  const [name, setName] = useState("");
  const set = <K extends keyof SubagentSettingsValue,>(key: K, value: SubagentSettingsValue[K]) => props.onChange({ ...props.value, [key]: value });
  const modelRouting = props.value.model_routing ?? { controller_model: "", controller_thinking: "medium", prompt: "", allowed_models: [] };
  const [allowedModelsDraft, setAllowedModelsDraft] = useState(() => listText(modelRouting.allowed_models));
  const updateModelRouting = <K extends keyof typeof modelRouting>(key: K, value: (typeof modelRouting)[K]) => {
    set("model_routing", { ...modelRouting, [key]: value });
  };
  useEffect(() => {
    if (JSON.stringify(parseList(allowedModelsDraft)) !== JSON.stringify(modelRouting.allowed_models)) {
      setAllowedModelsDraft(listText(modelRouting.allowed_models));
    }
  }, [allowedModelsDraft, modelRouting.allowed_models]);
  const profiles = props.value.profiles ?? {};
  const updateProfile = (profileName: string, profile: AgentProfile) => set("profiles", { ...profiles, [profileName]: profile });
  const removeProfile = (profileName: string) => {
    const next = { ...profiles };
    delete next[profileName];
    set("profiles", next);
  };
  const add = () => {
    const normalized = name.trim();
    if (!normalized || profiles[normalized]) return;
    set("profiles", { ...profiles, [normalized]: { description: "", thinking: "medium", tools: null, can_spawn: false } });
    setName("");
  };
  return (
    <SettingsSection
      id="subagents"
      eyebrow="DELEGATION"
      title="Subagents"
      description="Bound agent trees and define reusable profiles for independent work, exploration and review."
    >
      <Toggle label="Enable subagents" hint="Allow conversations to create managed child agents." checked={props.value.enabled} onChange={(enabled) => set("enabled", enabled)} />
      <div className="settings-grid three subagent-limits">
        <Field label="Maximum depth"><input type="number" min={1} value={props.value.max_depth} onChange={(event) => set("max_depth", Number(event.target.value))} /></Field>
        <Field label="Children per agent"><input type="number" min={1} value={props.value.max_children} onChange={(event) => set("max_children", Number(event.target.value))} /></Field>
        <Field label="Concurrent agents"><input type="number" min={1} value={props.value.max_concurrent} onChange={(event) => set("max_concurrent", Number(event.target.value))} /></Field>
      </div>
      <div className="toggle-grid">
        <Toggle label="Sibling messages" checked={props.value.allow_sibling_messages} onChange={(value) => set("allow_sibling_messages", value)} />
        <Toggle label="Auto-report results" checked={props.value.auto_report} onChange={(value) => set("auto_report", value)} />
      </div>
      <div className="integration-stack">
        <article className="integration-card compact">
          <header>
            <div className="integration-icon"><Bot size={16} /></div>
            <div><strong>Model router</strong><span>A controller decomposes work across allowed models.</span></div>
          </header>
          <div className="settings-grid two">
            <Field label="Controller model">
              <input value={modelRouting.controller_model} onChange={(event) => updateModelRouting("controller_model", event.target.value)} />
            </Field>
            <Field label="Controller thinking">
              <select value={modelRouting.controller_thinking} onChange={(event) => updateModelRouting("controller_thinking", event.target.value as typeof modelRouting.controller_thinking)}>
                <option value="">Inherit parent conversation</option>
                {configurationThinkingLevels.map((level) => <option key={level}>{level}</option>)}
              </select>
            </Field>
            <Field label="Allowed models">
              <textarea rows={4} value={allowedModelsDraft} onChange={(event) => {
                setAllowedModelsDraft(event.target.value);
                updateModelRouting("allowed_models", parseList(event.target.value));
              }} />
            </Field>
            <Field label="Routing prompt" wide>
              <textarea rows={5} value={modelRouting.prompt} onChange={(event) => updateModelRouting("prompt", event.target.value)} />
            </Field>
          </div>
        </article>
      </div>
      <div className="inline-create">
        <label><span>Profile name</span><input value={name} onChange={(event) => setName(event.target.value)} placeholder="security-review" /></label>
        <button className="secondary-button" onClick={add} disabled={!name.trim() || Boolean(profiles[name.trim()])}><Plus size={14} /> Add profile</button>
      </div>
      <div className="integration-stack">
        {Object.entries(profiles).map(([profileName, profile]) => (
          <ProfileCard key={profileName} name={profileName} value={profile} onChange={(value) => updateProfile(profileName, value)} onRemove={() => removeProfile(profileName)} />
        ))}
        {!Object.keys(profiles).length && <div className="integration-empty"><Users size={20} /><strong>No agent profiles</strong><span>Create a reusable role for spawned agents.</span></div>}
      </div>
    </SettingsSection>
  );
}

function ProfileCard(props: { name: string; value: AgentProfile; onChange: (value: AgentProfile) => void; onRemove: () => void }) {
  const set = <K extends keyof AgentProfile,>(key: K, value: AgentProfile[K]) => props.onChange({ ...props.value, [key]: value });
  const inheritsTools = props.value.tools == null;
  return (
    <article className="integration-card compact">
      <header>
        <div className="integration-icon"><Users size={16} /></div>
        <div><strong>{props.name}</strong><span>{props.value.model || "inherits model"} · {props.value.can_spawn ? "can spawn" : "leaf agent"}</span></div>
        <button className="icon-button danger-icon" onClick={props.onRemove} aria-label={`Remove ${props.name}`}><Trash2 size={14} /></button>
      </header>
      <div className="settings-grid two">
        <Field label="Description" wide><input value={props.value.description} onChange={(event) => set("description", event.target.value)} /></Field>
        <Field label="Model"><input value={props.value.model || ""} onChange={(event) => set("model", event.target.value)} placeholder="Inherit" /></Field>
        <Field label="Thinking"><select value={props.value.thinking || "medium"} onChange={(event) => set("thinking", event.target.value as AgentProfile["thinking"])}>{configurationThinkingLevels.map((level) => <option key={level}>{level}</option>)}</select></Field>
        <Field label="Instructions" wide><textarea rows={3} value={props.value.instructions || ""} onChange={(event) => set("instructions", event.target.value)} /></Field>
        <Field label="Tools" hint={inheritsTools ? "Inheriting parent tools." : "One per line; empty means no tools."} wide>
          <textarea rows={3} disabled={inheritsTools} value={listText(props.value.tools ?? [])} onChange={(event) => set("tools", parseList(event.target.value))} />
        </Field>
      </div>
      <div className="toggle-grid">
        <Toggle label="Inherit parent tools" checked={inheritsTools} onChange={(value) => set("tools", value ? null : [])} />
        <Toggle label="Can spawn children" checked={props.value.can_spawn} onChange={(value) => set("can_spawn", value)} />
      </div>
    </article>
  );
}
