import { FolderOpen, MessageSquarePlus, Plus, Settings2, X } from "lucide-react";
import { useEffect, useState } from "react";
import type { ConnectionStatus } from "../lib/protocol";
import { parseAdditionalFolders } from "../lib/sandbox-folders";

interface CreateDialogProps {
  kind: "chat" | "project";
  busy?: boolean;
  initialFolder?: string;
  initialCategory?: string;
  onClose: () => void;
  onCreate: (values: { name: string; cwd?: string; category?: string; additionalFolders?: string[] }) => Promise<void>;
}

export function CreateDialog(props: CreateDialogProps) {
  const [name, setName] = useState("");
  const [folder, setFolder] = useState(props.initialFolder || "");
  const [category, setCategory] = useState(props.initialCategory || "");
  const [additionalFolders, setAdditionalFolders] = useState("");
  useEscape(props.onClose);
  const project = props.kind === "project";
  const title = project ? "New project" : "New chat";
  return (
    <div className="modal-layer" role="dialog" aria-modal="true" aria-label={`Create ${props.kind}`}>
      <button className="modal-scrim" onClick={props.onClose} aria-label={`Close create ${props.kind}`} />
      <form
        className="modal-card"
        onSubmit={(event) => {
          event.preventDefault();
          void props.onCreate({
            name: name.trim(),
            cwd: project ? folder.trim() : undefined,
            category: project ? category.trim() : undefined,
            additionalFolders: project ? parseAdditionalFolders(additionalFolders) : undefined,
          });
        }}
      >
        <div className="modal-heading">
          <div className="modal-icon">{project ? <FolderOpen size={18} /> : <MessageSquarePlus size={18} />}</div>
          <div><strong>{title}</strong><span>{project ? "Give the agent a sandboxed folder" : "Start a pathless conversation"}</span></div>
          <button type="button" className="icon-button" onClick={props.onClose} aria-label="Close"><X size={17} /></button>
        </div>
        <label>
          <span>{project ? "Project name" : "Chat name"}</span>
          <input
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={project ? "My project" : "New idea"}
          />
        </label>
        {project && (
          <>
            <label>
              <span>Project category</span>
              <input
                value={category}
                onChange={(event) => setCategory(event.target.value)}
                placeholder="Client or workspace"
                aria-label="Project category"
                maxLength={80}
              />
            </label>
            <label>
              <span>Project folder</span>
              <input
                value={folder}
                onChange={(event) => setFolder(event.target.value)}
                placeholder="/absolute/path/to/project"
                spellCheck={false}
              />
            </label>
            <label>
              <span>Additional sandbox folders</span>
              <textarea
                value={additionalFolders}
                onChange={(event) => setAdditionalFolders(event.target.value)}
                placeholder={"/absolute/path/to/shared\n/absolute/path/to/docs"}
                aria-label="Additional sandbox folders"
                rows={3}
                spellCheck={false}
              />
            </label>
          </>
        )}
        <p>{project
          ? "The project folder remains the main working directory. Add one optional absolute folder per line."
          : "Chats retain their own SQLite history but cannot read or modify local files."}</p>
        <div className="modal-actions">
          <button type="button" className="secondary-button" onClick={props.onClose}>Cancel</button>
          <button
            type="submit"
            className="primary-button"
            disabled={!name.trim() || (project && !folder.trim()) || props.busy}
          >
            <Plus size={14} /> {props.busy ? "Creating…" : `Create ${props.kind}`}
          </button>
        </div>
      </form>
    </div>
  );
}

interface ConnectionDialogProps {
  endpoint: string;
  status: ConnectionStatus;
  error: string;
  onClose: () => void;
  onSave: (endpoint: string) => void;
}

export function ConnectionDialog(props: ConnectionDialogProps) {
  const [draft, setDraft] = useState(props.endpoint);
  const [validation, setValidation] = useState("");
  useEscape(props.onClose);
  const save = () => {
    const value = draft.trim();
    if (!/^wss?:\/\//.test(value)) {
      setValidation("Use a ws:// or wss:// WebSocket URL.");
      return;
    }
    props.onSave(value);
  };
  return (
    <div className="modal-layer" role="dialog" aria-modal="true" aria-label="Connection settings">
      <button className="modal-scrim" onClick={props.onClose} aria-label="Close connection settings" />
      <div className="modal-card">
        <div className="modal-heading">
          <div className="modal-icon"><Settings2 size={18} /></div>
          <div><strong>Daemon connection</strong><span>WebSocket endpoint for this browser</span></div>
          <button className="icon-button" onClick={props.onClose} aria-label="Close"><X size={17} /></button>
        </div>
        <label>
          <span>WebSocket URL</span>
          <input value={draft} onChange={(event) => setDraft(event.target.value)} spellCheck={false} />
        </label>
        <div className="connection-detail">
          <span className={`connection-dot ${props.status}`} />
          <span>{props.status === "online" ? "Connected" : props.error || "Not connected"}</span>
        </div>
        {validation && <p className="form-error">{validation}</p>}
        <p>Keep the same-origin <code>/ws</code> URL when Vite proxies your local daemon.</p>
        <div className="modal-actions">
          <button className="secondary-button" onClick={props.onClose}>Cancel</button>
          <button className="primary-button" onClick={save}>Reconnect</button>
        </div>
      </div>
    </div>
  );
}

function useEscape(onClose: () => void) {
  useEffect(() => {
    const listener = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, [onClose]);
}
