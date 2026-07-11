import type {
  Command,
  ConnectionStatus,
  ResponseEnvelope,
  WireEvent,
} from "./protocol";

type PendingRequest = {
  resolve: (data: unknown) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};

type WebSocketFactory = (url: string) => WebSocket;
type EventListener = (event: WireEvent) => void;
type StatusListener = (status: ConnectionStatus, error?: Error) => void;

let fallbackRequestID = 0;

function requestID(): string {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  fallbackRequestID += 1;
  return `web-${Date.now()}-${fallbackRequestID}`;
}

export class DaemonTransport {
  private socket?: WebSocket;
  private pending = new Map<string, PendingRequest>();
  private eventListeners = new Set<EventListener>();
  private statusListeners = new Set<StatusListener>();
  private manuallyClosed = false;

  constructor(
    readonly url: string,
    private readonly createSocket: WebSocketFactory = (value) => new WebSocket(value),
  ) {}

  get isOpen(): boolean {
    return this.socket?.readyState === WebSocket.OPEN;
  }

  get wasManuallyClosed(): boolean {
    return this.manuallyClosed;
  }

  onEvent(listener: EventListener): () => void {
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }

  onStatus(listener: StatusListener): () => void {
    this.statusListeners.add(listener);
    return () => this.statusListeners.delete(listener);
  }

  async connect(): Promise<void> {
    if (this.isOpen) return;
    this.manuallyClosed = false;
    this.emitStatus("connecting");
    await new Promise<void>((resolve, reject) => {
      const socket = this.createSocket(this.url);
      this.socket = socket;
      let settled = false;
      const failConnect = () => {
        if (settled) return;
        settled = true;
        const error = new Error(`Unable to connect to ${this.url}`);
        this.emitStatus("offline", error);
        reject(error);
      };
      socket.addEventListener("open", () => {
        if (settled) return;
        settled = true;
        this.emitStatus("online");
        resolve();
      }, { once: true });
      socket.addEventListener("error", failConnect, { once: true });
      socket.addEventListener("message", (event) => this.handleMessage(event));
      socket.addEventListener("close", () => this.handleClose());
    });
  }

  close(): void {
    this.manuallyClosed = true;
    this.socket?.close(1000, "client closed");
    this.socket = undefined;
    this.rejectPending(new Error("Daemon connection closed"));
    this.emitStatus("offline");
  }

  request<T>(command: Command, timeoutMs = 15_000): Promise<T> {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error("Daemon is not connected"));
    }
    const id = command.id || requestID();
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`${command.type} timed out`));
      }, timeoutMs);
      this.pending.set(id, { resolve: (data) => resolve(data as T), reject, timer });
      try {
        this.socket?.send(JSON.stringify({ ...command, id }));
      } catch (error) {
        clearTimeout(timer);
        this.pending.delete(id);
        reject(error instanceof Error ? error : new Error(String(error)));
      }
    });
  }

  private handleMessage(event: MessageEvent): void {
    try {
      const message = JSON.parse(String(event.data)) as ResponseEnvelope | WireEvent;
      if (message.type === "response") {
        const response = message as ResponseEnvelope;
        const pending = this.pending.get(response.id);
        if (!pending) return;
        clearTimeout(pending.timer);
        this.pending.delete(response.id);
        if (response.success) pending.resolve(response.data);
        else pending.reject(new Error(response.error || `${response.command} failed`));
        return;
      }
      for (const listener of this.eventListeners) listener(message as WireEvent);
    } catch (error) {
      this.emitStatus("offline", error instanceof Error ? error : new Error("Invalid daemon message"));
    }
  }

  private handleClose(): void {
    this.socket = undefined;
    const error = new Error("Daemon connection lost");
    this.rejectPending(error);
    this.emitStatus("offline", this.manuallyClosed ? undefined : error);
  }

  private rejectPending(error: Error): void {
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer);
      pending.reject(error);
    }
    this.pending.clear();
  }

  private emitStatus(status: ConnectionStatus, error?: Error): void {
    for (const listener of this.statusListeners) listener(status, error);
  }
}
