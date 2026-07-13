import { FolderOpen, MessageSquarePlus, Plus, Settings2, X } from "lucide-react";
import { useEffect, useId, useRef, useState, type ChangeEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type {
  ConnectionStatus,
  ProjectWorkspaceInspection,
} from "../lib/protocol";
import { parseAdditionalFolders } from "../lib/sandbox-folders";

export interface CreateConversationValues {
  name: string;
  cwd?: string;
  category?: string;
  additionalFolders?: string[];
  worktree?: { baseRef?: string; environmentID?: string; sourceFolder?: string };
}

interface CreateDialogProps {
  kind: "chat" | "project";
  busy?: boolean;
  initialFolder?: string;
  initialCategory?: string;
  onCompleteFolder?: (path: string) => Promise<string[]>;
  onClose: () => void;
  onCreate: (values: CreateConversationValues) => Promise<void>;
  onInspectWorkspace?: (folder: string) => Promise<ProjectWorkspaceInspection>;
}

export function CreateDialog(props: CreateDialogProps) {
  const [name, setName] = useState("");
  const [folder, setFolder] = useState(props.initialFolder || "");
  const [category, setCategory] = useState(props.initialCategory || "");
  const [additionalFolders, setAdditionalFolders] = useState("");
  const [workspaceMode, setWorkspaceMode] = useState<"local" | "worktree">("local");
  const [baseRef, setBaseRef] = useState("HEAD");
  const [environmentID, setEnvironmentID] = useState("");
  const [inspection, setInspection] = useState<ProjectWorkspaceInspection | null>(null);
  const [inspectedFolder, setInspectedFolder] = useState("");
  const [inspecting, setInspecting] = useState(false);
  const [inspectionError, setInspectionError] = useState("");
  useEscape(props.onClose);
  const project = props.kind === "project";
  const title = project ? "New project" : "New chat";
  const folderValue = folder.trim();
  const inspected = inspectedFolder === folderValue ? inspection : null;
  const worktreeReady = workspaceMode === "local" || Boolean(inspected?.git_repository);
  const inspectWorkspace = async () => {
    if (!folderValue || !props.onInspectWorkspace) return;
    setInspecting(true);
    setInspectionError("");
    try {
      const value = await props.onInspectWorkspace(folderValue);
      const next = {
        ...value,
        branches: value.branches ?? [],
        environments: value.environments ?? [],
      };
      setInspection(next);
      setInspectedFolder(folderValue);
      setEnvironmentID((current) => next.environments.some((environment) => environment.id === current) ? current : "");
      if (!next.git_repository) setInspectionError("Worktrees require a folder inside a Git repository.");
    } catch (cause) {
      setInspection(null);
      setInspectedFolder("");
      setInspectionError(cause instanceof Error ? cause.message : "Could not inspect this folder");
    } finally {
      setInspecting(false);
    }
  };
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
            worktree: project && workspaceMode === "worktree" ? {
              baseRef: baseRef.trim() || "HEAD",
              environmentID: environmentID || undefined,
              sourceFolder: inspected?.folder,
            } : undefined,
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
              <span>Workspace</span>
              <select
                aria-label="Project workspace"
                value={workspaceMode}
                onChange={(event) => {
                  setWorkspaceMode(event.target.value as "local" | "worktree");
                  setInspectionError("");
                }}
              >
                <option value="local">Local checkout</option>
                <option value="worktree">New worktree</option>
              </select>
            </label>
            <label>
              <span>{workspaceMode === "worktree" ? "Source project folder" : "Project folder"}</span>
              <FolderAutocomplete
                value={folder}
                onChange={(value) => {
                  setFolder(value);
                  setInspection(null);
                  setInspectedFolder("");
                  setInspectionError("");
                }}
                onComplete={props.onCompleteFolder}
                placeholder="/absolute/path/to/project"
              />
            </label>
            {workspaceMode === "worktree" && (
              <>
                <div className="project-inspection-row">
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={!folderValue || inspecting || props.busy}
                    onClick={() => void inspectWorkspace()}
                  >
                    {inspecting ? "Inspecting…" : "Inspect source folder"}
                  </button>
                  {inspected?.git_repository && (
                    <span>{inspected.repository_root || inspected.folder}{inspected.current_branch ? ` · ${inspected.current_branch}` : ""}</span>
                  )}
                </div>
                {inspectionError && <p className="form-error">{inspectionError}</p>}
                <label>
                  <span>Starting ref</span>
                  <input
                    list="worktree-ref-options"
                    aria-label="Starting ref"
                    value={baseRef}
                    onChange={(event) => setBaseRef(event.target.value)}
                    placeholder="HEAD"
                    spellCheck={false}
                  />
                  <datalist id="worktree-ref-options">
                    {inspected?.branches.map((branch) => <option value={branch} key={branch} />)}
                  </datalist>
                </label>
                <label>
                  <span>Local environment</span>
                  <select
                    aria-label="Local environment"
                    value={environmentID}
                    onChange={(event) => setEnvironmentID(event.target.value)}
                    disabled={!inspected?.git_repository}
                  >
                    <option value="">No environment</option>
                    {inspected?.environments.map((environment) => (
                      <option value={environment.id} key={environment.id}>{environment.name} · {environment.id}</option>
                    ))}
                  </select>
                </label>
              </>
            )}
            <label>
              <span>Additional sandbox folders</span>
              <FolderAutocomplete
                multiline
                value={additionalFolders}
                onChange={setAdditionalFolders}
                onComplete={props.onCompleteFolder}
                placeholder={"/absolute/path/to/shared\n/absolute/path/to/docs"}
                aria-label="Additional sandbox folders"
              />
            </label>
          </>
        )}
        <p>{project
          ? workspaceMode === "worktree"
            ? "Dire Agent creates an isolated checkout, then runs the selected environment setup script before opening the project."
            : "The project folder remains the main working directory. Add one optional absolute folder per line."
          : "Chats retain their own SQLite history but cannot read or modify local files."}</p>
        {project && workspaceMode === "worktree" && props.busy && (
          <p className="project-creation-status" role="status">Creating the worktree and running its setup script…</p>
        )}
        <div className="modal-actions">
          <button type="button" className="secondary-button" onClick={props.onClose}>Cancel</button>
          <button
            type="submit"
            className="primary-button"
            disabled={!name.trim() || (project && !folder.trim()) || (project && !worktreeReady) || props.busy}
          >
            <Plus size={14} /> {props.busy
              ? workspaceMode === "worktree" ? "Creating worktree…" : "Creating…"
              : `Create ${props.kind}`}
          </button>
        </div>
      </form>
    </div>
  );
}

interface FolderAutocompleteProps {
  value: string;
  onChange: (value: string) => void;
  onComplete?: (path: string) => Promise<string[]>;
  placeholder: string;
  multiline?: boolean;
  "aria-label"?: string;
}

function FolderAutocomplete(props: FolderAutocompleteProps) {
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [active, setActive] = useState(0);
  const [open, setOpen] = useState(false);
  const request = useRef(0);
  const acceptedPath = useRef("");
  const listID = useId();
  const currentPath = props.multiline ? props.value.split("\n").at(-1) ?? "" : props.value;

  useEffect(() => {
    if (currentPath === acceptedPath.current) {
      acceptedPath.current = "";
      setSuggestions([]);
      setOpen(false);
      return;
    }
    if (!props.onComplete || (!currentPath.startsWith("/") && !currentPath.startsWith("~"))) {
      setSuggestions([]);
      setOpen(false);
      return;
    }
    const requestID = ++request.current;
    const timer = window.setTimeout(() => {
      void props.onComplete!(currentPath).then((folders) => {
        if (request.current !== requestID) return;
        setSuggestions(folders);
        setActive(0);
        setOpen(folders.length > 0);
      }).catch(() => {
        if (request.current === requestID) setOpen(false);
      });
    }, 120);
    return () => window.clearTimeout(timer);
  }, [currentPath, props.onComplete]);

  const choose = (folder: string) => {
    acceptedPath.current = folder;
    const value = props.multiline
      ? [...props.value.split("\n").slice(0, -1), folder].join("\n")
      : folder;
    props.onChange(value);
    setOpen(false);
  };
  const onKeyDown = (event: ReactKeyboardEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    if (!open || suggestions.length === 0) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const direction = event.key === "ArrowDown" ? 1 : -1;
      setActive((index) => (index + direction + suggestions.length) % suggestions.length);
    } else if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      choose(suggestions[active]);
    } else if (event.key === "Escape") {
      event.stopPropagation();
      setOpen(false);
    }
  };
  const shared = {
    value: props.value,
    onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => props.onChange(event.target.value),
    onKeyDown,
    onFocus: () => suggestions.length > 0 && setOpen(true),
    placeholder: props.placeholder,
    spellCheck: false,
    role: "combobox",
    "aria-label": props["aria-label"],
    "aria-autocomplete": "list" as const,
    "aria-expanded": open,
    "aria-controls": open ? listID : undefined,
    "aria-activedescendant": open ? `${listID}-${active}` : undefined,
  };
  return (
    <div className="folder-autocomplete">
      {props.multiline ? <textarea {...shared} rows={3} /> : <input {...shared} />}
      {open && (
        <div className="folder-suggestions" id={listID} role="listbox" aria-label="Folder suggestions">
          {suggestions.map((folder, index) => (
            <button
              type="button"
              id={`${listID}-${index}`}
              role="option"
              aria-selected={index === active}
              className={index === active ? "active" : ""}
              key={folder}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(folder)}
            >{folder}</button>
          ))}
        </div>
      )}
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
