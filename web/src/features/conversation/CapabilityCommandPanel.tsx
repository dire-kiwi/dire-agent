import { Play, RefreshCw, TerminalSquare } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { CapabilityCommandController } from "../../hooks/useCapabilityCommands";

export function CapabilityCommandPanel({ controller }: { controller: CapabilityCommandController }) {
  const [selectedName, setSelectedName] = useState("");
  const [commandArguments, setCommandArguments] = useState("");
  useEffect(() => {
    if (!controller.commands.some((command) => command.name === selectedName)) {
      setSelectedName(controller.commands[0]?.name || "");
    }
  }, [controller.commands, selectedName]);
  const selected = useMemo(
    () => controller.commands.find((command) => command.name === selectedName),
    [controller.commands, selectedName],
  );

  const execute = async () => {
    if (await controller.execute(selectedName, commandArguments)) setCommandArguments("");
  };
  return (
    <section className="drawer-section capability-command-panel">
      <div className="section-title">
        <span>Extension commands</span>
        <button className="tiny-button" onClick={() => void controller.refresh()} disabled={controller.loading}>
          <RefreshCw className={controller.loading ? "spin" : ""} size={11} /> Refresh
        </button>
      </div>
      {!controller.supported ? (
        <p className="quiet-copy">This daemon does not expose capability commands.</p>
      ) : controller.loading && !controller.commands.length ? (
        <p className="quiet-copy">Discovering extension commands…</p>
      ) : controller.commands.length ? (
        <div className="capability-command-form">
          <label>
            <span>Command</span>
            <select
              aria-label="Extension command"
              value={selectedName}
              onChange={(event) => {
                setSelectedName(event.target.value);
                setCommandArguments("");
                controller.clearResult();
              }}
            >
              {controller.commands.map((command) => (
                <option value={command.name} key={`${command.source}:${command.name}`}>/{command.name}</option>
              ))}
            </select>
          </label>
          {selected && (
            <div className="command-description">
              <TerminalSquare size={13} />
              <span><strong>/{selected.name}</strong><small>{selected.description || "Extension command"}{selected.source ? ` · ${selected.source}` : ""}</small></span>
            </div>
          )}
          <label>
            <span>Arguments</span>
            <textarea
              rows={2}
              value={commandArguments}
              onChange={(event) => setCommandArguments(event.target.value)}
              aria-label={`Arguments for /${selectedName}`}
              placeholder="Optional command arguments"
            />
          </label>
          <button className="secondary-button full-width" onClick={() => void execute()} disabled={!selectedName || controller.executing}>
            {controller.executing ? <RefreshCw className="spin" size={13} /> : <Play size={13} />}
            {controller.executing ? "Running…" : `Run /${selectedName}`}
          </button>
          {(controller.result || controller.error) && (
            <div className={`command-result ${controller.error ? "failed" : ""}`} role="status">
              <strong>{controller.error ? "Command failed" : "Command complete"}</strong>
              {(controller.error || controller.result?.output) && <pre>{controller.error || controller.result?.output}</pre>}
              {controller.result?.prompt && <span>Prompt queued for the agent.</span>}
            </div>
          )}
        </div>
      ) : (
        <p className="quiet-copy">No extension commands are available for this conversation.</p>
      )}
    </section>
  );
}
