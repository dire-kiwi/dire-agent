import { Bot, ChevronRight, CircleStop, Clock3, MessageSquare, Plus, Send, Trash2, Users, X } from "lucide-react";
import { useMemo, useState, type KeyboardEvent } from "react";
import type { SubagentController } from "../../hooks/useSubagents";
import { mergeModelOptions } from "../../lib/display";
import type { ModelInfo, SpawnAgentOptions, SubagentInfo } from "../../lib/protocol";

export function SubagentPanel(props: { controller: SubagentController; models: ModelInfo[] }) {
  const { controller } = props;
  const [creating, setCreating] = useState(false);
  const [message, setMessage] = useState("");
  const children = useMemo(
    () => controller.selected ? controller.agents.filter((item) => item.parent_id === controller.selected?.id) : [],
    [controller.agents, controller.selected],
  );
  const parent = controller.selected?.parent_id
    ? controller.agents.find((item) => item.id === controller.selected?.parent_id)
    : null;

  if (!controller.supported) {
    return (
      <section className="drawer-section">
        <div className="section-title"><span>Subagents</span><small>Unavailable</small></div>
        <p className="quiet-copy">This daemon does not expose the subagent API yet.</p>
      </section>
    );
  }

  const send = async () => {
    if (await controller.send(message)) setMessage("");
  };
  return (
    <section className="drawer-section subagent-panel">
      <div className="section-title">
        <span>Subagents</span>
        <button className="tiny-button" onClick={() => setCreating((value) => !value)}>
          {creating ? <X size={12} /> : <Plus size={12} />}{creating ? "Cancel" : "Spawn"}
        </button>
      </div>
      {creating && (
        <SpawnAgentForm
          parent={controller.selected}
          models={props.models}
          onSubmit={async (options) => {
            if (await controller.spawn(options)) setCreating(false);
          }}
        />
      )}
      <div className="agent-tree" aria-label="Subagent tree">
        {controller.agents.map((agent) => (
          <button
            key={agent.id}
            className={controller.selected?.id === agent.id ? "selected" : ""}
            style={{ paddingLeft: `${9 + Math.max(0, agent.depth) * 14}px` }}
            onClick={() => void controller.select(agent.id)}
          >
            <span className={`agent-status ${statusClass(agent.status)}`} />
            <span><strong>{agent.agent_name}</strong><small>{agent.depth === 0 ? "root conversation" : agent.agent_role || agent.profile || "general"} · {agent.status}</small></span>
            <ChevronRight size={12} />
          </button>
        ))}
        {!controller.agents.length && !controller.loading && (
          <div className="agent-empty"><Users size={18} /><span>No child agents yet</span></div>
        )}
        {controller.loading && <p className="quiet-copy">Loading agents…</p>}
      </div>

      {controller.selected && (
        <div className="agent-detail">
          <header>
            <div className="agent-avatar"><Bot size={14} /></div>
            <div><strong>{controller.selected.agent_name}</strong><span>{controller.selected.model || "inherited model"} · depth {controller.selected.depth}</span></div>
            <button
              className="icon-button"
              onClick={() => void controller.wait()}
              disabled={controller.waiting}
              aria-label={`Wait for ${controller.selected.agent_name}`}
              title="Wait up to 10 seconds"
            ><Clock3 className={controller.waiting ? "pulse" : ""} size={14} /></button>
            {controller.selected.status === "running" && (
              <button className="icon-button" onClick={() => void controller.interrupt()} aria-label={`Interrupt ${controller.selected.agent_name}`}><CircleStop size={14} /></button>
            )}
            {controller.selected.depth > 0 && controller.selected.status !== "running" && (
              <button
                className="icon-button"
                onClick={() => window.confirm(`Delete agent “${controller.selected?.agent_name}”?`) && void controller.remove()}
                aria-label={`Delete ${controller.selected.agent_name}`}
              ><Trash2 size={14} /></button>
            )}
          </header>
          {controller.selected.task && <p className="agent-task">{controller.selected.task}</p>}
          {(parent || children.length > 0) && (
            <div className="agent-relations">
              {parent && <button onClick={() => void controller.select(parent.id)}>Parent: {parent.agent_name}</button>}
              {children.map((child) => <button key={child.id} onClick={() => void controller.select(child.id)}>Child: {child.agent_name}</button>)}
            </div>
          )}
          <div className="agent-transcript" aria-label={`${controller.selected.agent_name} transcript`}>
            {controller.messages.map((item, index) => (
              <article className={item.role === "user" ? "outbound" : "inbound"} key={item.id || `${item.created_at}-${index}`}>
                <small>{item.role === "user" ? "You" : controller.selected?.agent_name}</small>
                <p>{item.content}</p>
              </article>
            ))}
            {!controller.messages.length && <div className="agent-empty"><MessageSquare size={16} /><span>No messages yet</span></div>}
          </div>
          <div className="agent-composer">
            <textarea
              value={message}
              onChange={(event) => setMessage(event.target.value)}
              onKeyDown={(event: KeyboardEvent<HTMLTextAreaElement>) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  void send();
                }
              }}
              rows={2}
              aria-label={`Message ${controller.selected.agent_name}`}
              placeholder="Send guidance to this agent…"
            />
            <button onClick={() => void send()} disabled={!message.trim()} aria-label="Send agent message"><Send size={13} /></button>
          </div>
        </div>
      )}
    </section>
  );
}

function SpawnAgentForm(props: { parent: SubagentInfo | null; models: ModelInfo[]; onSubmit: (options: SpawnAgentOptions) => Promise<void> }) {
  const [name, setName] = useState("");
  const [task, setTask] = useState("");
  const [role, setRole] = useState("general");
  const [mode, setMode] = useState<NonNullable<SpawnAgentOptions["mode"]>>("direct");
  const [model, setModel] = useState("");
  const options = mergeModelOptions(props.models);
  return (
    <form className="spawn-agent-form" aria-label="Spawn agent" onSubmit={(event) => {
      event.preventDefault();
      void props.onSubmit({
        parent_id: props.parent?.id,
        agent_name: name.trim(),
        agent_role: role.trim(),
        profile: role.trim(),
        task: task.trim(),
        mode,
        model: mode === "direct" ? model || undefined : undefined,
      });
    }}>
      <label><span>Spawn mode</span><select value={mode} onChange={(event) => setMode(event.target.value as NonNullable<SpawnAgentOptions["mode"]>)}><option value="direct">Direct model</option><option value="model-router">Choose model automatically</option></select></label>
      <div className="field-grid">
        <label><span>Name</span><input value={name} onChange={(event) => setName(event.target.value)} placeholder="reviewer" /></label>
        <label><span>Role/profile</span><input value={role} onChange={(event) => setRole(event.target.value)} placeholder="general" /></label>
      </div>
      <label><span>Task</span><textarea rows={3} value={task} onChange={(event) => setTask(event.target.value)} placeholder="Review the authentication flow…" /></label>
      {mode === "direct" ? (
        <label><span>Model</span><select value={model} onChange={(event) => setModel(event.target.value)}><option value="">Inherit parent</option>{options.map((item) => <option value={item.id} key={item.id}>{item.id}</option>)}</select></label>
      ) : (
        <p className="quiet-copy">The configured controller will choose and start one or more workers on allowed models.</p>
      )}
      <button className="secondary-button full-width" type="submit" disabled={!name.trim() || !task.trim()}><Plus size={13} /> Spawn {props.parent ? `under ${props.parent.agent_name}` : "child agent"}</button>
    </form>
  );
}

function statusClass(status: string): string {
  if (status === "running") return "running";
  if (status === "error" || status === "failed" || status === "interrupted") return "error";
  if (status === "completed") return "completed";
  return "idle";
}
