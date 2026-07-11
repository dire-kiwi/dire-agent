import { FitAddon } from "@xterm/addon-fit";
import { LigaturesAddon } from "@xterm/addon-ligatures/lib/addon-ligatures.mjs";
import { WebglAddon } from "@xterm/addon-webgl";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { useEffect, useRef, useState } from "react";
import type { ProjectLauncher } from "../lib/configuration";
import type { Conversation } from "../lib/protocol";
import {
  terminalFallbackLigatures,
  terminalFontFamily,
  terminalIconFont,
  terminalIconProbe,
  terminalLigatureFeatures,
  terminalPrimaryFont,
  terminalTheme,
  terminalWebSocketURL,
} from "../lib/terminal";

interface TerminalPanelProps {
  endpoint: string;
  project: Conversation;
  launcher: ProjectLauncher;
  active: boolean;
}

const noResize = () => undefined;

export function TerminalPanel(props: TerminalPanelProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAndResizeRef = useRef<() => void>(noResize);
  const activeRef = useRef(props.active);
  const [status, setStatus] = useState("Connecting…");

  activeRef.current = props.active;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    setStatus("Connecting…");
    const terminal = new Terminal({
      allowProposedApi: true,
      convertEol: false,
      cursorBlink: true,
      cursorStyle: "block",
      customGlyphs: true,
      drawBoldTextInBrightColors: false,
      fontFamily: terminalFontFamily,
      fontSize: 13,
      fontWeight: "400",
      fontWeightBold: "600",
      letterSpacing: 0,
      lineHeight: 1.2,
      minimumContrastRatio: 1,
      scrollback: 10_000,
      theme: terminalTheme,
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host);
    terminalRef.current = terminal;

    host.dataset.ligatures = "loading";
    try {
      terminal.loadAddon(new LigaturesAddon({
        fallbackLigatures: terminalFallbackLigatures,
        fontFeatureSettings: terminalLigatureFeatures,
      }));
      host.dataset.ligatures = "enabled";
    } catch (error) {
      host.dataset.ligatures = "unavailable";
      host.dataset.ligatureError = error instanceof Error ? error.message : String(error);
    }

    let webgl: WebglAddon | null = null;
    let contextLoss: { dispose: () => void } = { dispose: () => undefined };
    try {
      webgl = new WebglAddon();
      contextLoss = webgl.onContextLoss(() => {
        webgl?.dispose();
        webgl = null;
        host.dataset.renderer = "dom";
        if (terminal.rows > 0) terminal.refresh(0, terminal.rows - 1);
      });
      terminal.loadAddon(webgl);
      host.dataset.renderer = "webgl";
    } catch (error) {
      contextLoss.dispose();
      webgl?.dispose();
      webgl = null;
      host.dataset.renderer = "dom";
      host.dataset.rendererError = error instanceof Error ? error.message : String(error);
    }

    let disposed = false;
    const socket = new WebSocket(terminalWebSocketURL(props.endpoint, props.project.id, props.launcher.id));

    const sendDimensions = () => {
      if (socket.readyState !== WebSocket.OPEN) return;
      try {
        socket.send(JSON.stringify({ type: "resize", cols: terminal.cols, rows: terminal.rows }));
      } catch {
        // A concurrent socket close will be reflected by the close listener.
      }
    };
    const fitAndResize = () => {
      if (disposed || !host.isConnected || host.clientWidth === 0 || host.clientHeight === 0) return;
      try {
        fit.fit();
        sendDimensions();
      } catch {
        // Layout can change between measurement and fitting. The observer retries.
      }
    };
    fitAndResizeRef.current = fitAndResize;

    const output = (value: string) => {
      const binary = atob(value);
      const bytes = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
      terminal.write(bytes);
    };

    socket.addEventListener("open", () => {
      setStatus("Connected");
      fitAndResize();
      if (activeRef.current) terminal.focus();
    });
    socket.addEventListener("message", (event) => {
      try {
        const message = JSON.parse(String(event.data)) as {
          type: string;
          data?: string;
          code?: number;
          message?: string;
        };
        if (message.type === "output" && message.data) output(message.data);
        if (message.type === "ready") setStatus("Connected");
        if (message.type === "error") {
          setStatus(message.message || "Terminal error");
          terminal.writeln(`\r\n\x1b[31m${message.message || "Terminal error"}\x1b[0m`);
        }
        if (message.type === "exit") {
          setStatus(`Exited${message.code ? ` (${message.code})` : ""}`);
          terminal.writeln(`\r\n\x1b[90m[process exited${message.code ? ` with code ${message.code}` : ""}]\x1b[0m`);
        }
      } catch {
        setStatus("Invalid terminal response");
      }
    });
    socket.addEventListener("error", () => {
      setStatus(`Could not open ${props.launcher.id}`);
      terminal.writeln("\r\n\x1b[31mCould not connect to the project terminal.\x1b[0m");
    });
    socket.addEventListener("close", () => {
      setStatus((current) => current === "Connected" ? "Closed" : current);
    });

    const input = terminal.onData((data) => {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "input", data }));
    });
    const resize = terminal.onResize(sendDimensions);
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(fitAndResize);
    observer?.observe(host);
    window.addEventListener("resize", fitAndResize);
    const frame = window.requestAnimationFrame(fitAndResize);

    const fontSet = document.fonts;
    if (typeof fontSet?.load === "function") {
      const refreshFonts = () => {
        if (disposed) return;
        terminal.clearTextureAtlas();
        if (terminal.rows > 0) terminal.refresh(0, terminal.rows - 1);
        fitAndResize();
      };
      void fontSet.load(`400 13px ${terminalPrimaryFont}`, "=>").then(refreshFonts).catch(() => undefined);
      host.dataset.icons = "loading";
      void fontSet.load(`400 13px ${terminalIconFont}`, terminalIconProbe).then((faces) => {
        if (disposed) return;
        host.dataset.icons = faces.length > 0 ? "enabled" : "unavailable";
        refreshFonts();
      }).catch((error: unknown) => {
        if (disposed) return;
        host.dataset.icons = "unavailable";
        host.dataset.iconError = error instanceof Error ? error.message : String(error);
      });
    }

    return () => {
      disposed = true;
      fitAndResizeRef.current = noResize;
      if (terminalRef.current === terminal) terminalRef.current = null;
      window.cancelAnimationFrame(frame);
      window.removeEventListener("resize", fitAndResize);
      observer?.disconnect();
      input.dispose();
      resize.dispose();
      contextLoss.dispose();
      socket.close(1000, "terminal panel closed");
      terminal.dispose();
    };
  }, [props.endpoint, props.launcher.id, props.project.id]);

  useEffect(() => {
    if (!props.active) {
      terminalRef.current?.blur();
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      fitAndResizeRef.current();
      const terminal = terminalRef.current;
      if (!terminal) return;
      if (terminal.rows > 0) terminal.refresh(0, terminal.rows - 1);
      terminal.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [props.active]);

  const connected = status === "Connected";
  return (
    <section
      className={`terminal-panel${props.active ? " terminal-panel-active" : ""}`}
      role="tabpanel"
      aria-hidden={!props.active}
      aria-label={`${props.launcher.id} for ${props.project.name || props.project.id}`}
      data-launcher-id={props.launcher.id}
    >
      <div
        ref={hostRef}
        className="terminal-host"
        role="application"
        aria-label={`${props.launcher.id} terminal`}
      />
      <div className={`terminal-panel-status${connected ? " connected" : ""}`} role="status">
        {status}
      </div>
    </section>
  );
}
