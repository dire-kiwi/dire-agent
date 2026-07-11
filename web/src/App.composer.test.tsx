import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockState, projectFixture, resetMockDaemon } from "./test/mock-daemon";

vi.mock("./lib/daemon-client", async () => {
  const mock = await import("./test/mock-daemon");
  return {
    DaemonClient: mock.MockDaemonClient,
    unsupported: (error: unknown) => /unknown command|unsupported/i.test(error instanceof Error ? error.message : String(error)),
  };
});

import App from "./App";

describe("conversation composer", () => {
  afterEach(() => cleanup());
  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
    mockState.projects = [projectFixture];
  });

  it("completes slash commands with the keyboard before executing them", async () => {
    const user = userEvent.setup();
    render(<App />);
    const composer = await screen.findByLabelText("Message the agent");

    await user.type(composer, "/thi");
    const suggestions = screen.getByRole("listbox", { name: "Slash command suggestions" });
    expect(suggestions).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /\/thinking/i })).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Tab}");
    expect(composer).toHaveValue("/thinking ");
    expect(screen.queryByRole("listbox", { name: "Slash command suggestions" })).not.toBeInTheDocument();

    await user.clear(composer);
    await user.type(composer, "/he");
    await user.keyboard("{Enter}");
    expect(composer).toHaveValue("/help");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Chat commands")).toBeInTheDocument();
  });

  it("shows and updates project sandbox folders with slash commands", async () => {
    const user = userEvent.setup();
    render(<App />);
    const composer = await screen.findByLabelText("Message the agent");

    await user.type(composer, "/folders{Enter}");
    expect(await screen.findByText("Main project folder")).toBeInTheDocument();
    expect(screen.getByText("/workspace")).toBeInTheDocument();

    await user.type(composer, "/folder-add /workspace-shared{Enter}");
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_project_sandbox_folders",
      project_id: projectFixture.id,
      additional_folders: ["/workspace-shared"],
    })));
  });

  it("changes model and thinking level from the composer bottom row", async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByLabelText("Message the agent");

    const model = screen.getByRole("combobox", { name: "Composer model" });
    await user.click(model);
    expect(model).toHaveAttribute("aria-expanded", "true");
    await user.click(within(screen.getByRole("listbox", { name: "Composer model options" })).getByRole("option", { name: "gpt-5.6-luna" }));

    const thinking = screen.getByRole("combobox", { name: "Composer thinking level" });
    await user.click(thinking);
    await user.click(within(screen.getByRole("listbox", { name: "Composer thinking level options" })).getByRole("option", { name: "high" }));

    await waitFor(() => {
      expect(mockState.requests).toContainEqual(expect.objectContaining({
        type: "set_model",
        project_id: projectFixture.id,
        model: "gpt-5.6-luna",
      }));
      expect(mockState.requests).toContainEqual(expect.objectContaining({
        type: "set_thinking_level",
        project_id: projectFixture.id,
        level: "high",
      }));
    });
  });

  it("navigates themed composer dropdowns with the keyboard", async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByLabelText("Message the agent");

    const behavior = screen.getByRole("combobox", { name: "Message behavior" });
    behavior.focus();
    await user.keyboard("{Enter}{ArrowDown}{ArrowDown}{Enter}");
    expect(behavior).toHaveTextContent("Follow-up");
    expect(behavior).toHaveAttribute("aria-expanded", "false");

    await user.keyboard("{Enter}{Escape}");
    expect(behavior).toHaveAttribute("aria-expanded", "false");
  });

  it("renders streamed thinking and complete tool input/output in the chat", async () => {
    render(<App />);
    await screen.findByLabelText("Message the agent");

    await act(async () => {
      const emit = (type: string, sequence: number, data: Record<string, unknown>) => {
        mockState.eventListeners.forEach((listener) => listener({
          type,
          sequence,
          conversation_id: projectFixture.id,
          project_id: projectFixture.id,
          thread_id: projectFixture.id,
          timestamp: `2026-07-11T00:00:0${sequence}Z`,
          data,
        }));
      };
      emit("reasoning_start", 1, { message_id: "reasoning-1" });
      emit("reasoning_update", 2, { message_id: "reasoning-1", delta: "Inspecting the project." });
      emit("reasoning_end", 3, { message_id: "reasoning-1", text: "Inspecting the project." });
      emit("tool_execution_start", 4, {
        tool_call_id: "tool-1",
        tool_name: "read",
        arguments: { path: "README.md" },
      });
    });

    expect(await screen.findByText("Inspecting the project.")).toBeInTheDocument();
    expect(screen.getByText("Running read")).toBeInTheDocument();

    await act(async () => {
      mockState.eventListeners.forEach((listener) => listener({
        type: "tool_execution_end",
        sequence: 5,
        conversation_id: projectFixture.id,
        project_id: projectFixture.id,
        thread_id: projectFixture.id,
        timestamp: "2026-07-11T00:00:05Z",
        data: {
          tool_call_id: "tool-1",
          tool_name: "read",
          arguments: { path: "README.md" },
          output: "project documentation",
        },
      }));
    });

    expect((await screen.findAllByText("Input")).length).toBeGreaterThan(0);
    expect(screen.getByText(/README\.md/)).toBeInTheDocument();
    expect(screen.getByText("project documentation")).toBeInTheDocument();
  });

  it("pastes, previews, and sends an image through the project sandbox command", async () => {
    const user = userEvent.setup();
    render(<App />);
    const composer = await screen.findByLabelText("Message the agent");
    const image = new File([new Uint8Array([1, 2, 3, 4])], "clipboard.png", { type: "image/png" });

    fireEvent.paste(composer, { clipboardData: { files: [image] } });
    expect(await screen.findByRole("img", { name: "clipboard.png" })).toHaveAttribute("src", "data:image/png;base64,AQIDBA==");
    await user.type(composer, "What is in this image?");
    await user.click(screen.getByRole("button", { name: "Send message" }));

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "prompt",
      message: "What is in this image?",
      attachments: [expect.objectContaining({
        name: "clipboard.png", mime_type: "image/png", data: "AQIDBA==", size: 4,
      })],
    })));
    expect(await screen.findByRole("img", { name: "clipboard.png" })).toBeInTheDocument();
  });

  it("rejects pasted images in pathless chats", async () => {
    mockState.projects = [];
    mockState.chats = [{ ...projectFixture, id: "chat_image", kind: "chat", cwd: "", tools: [] }];
    render(<App />);
    const composer = await screen.findByLabelText("Message the agent");
    const image = new File([new Uint8Array([1])], "clipboard.png", { type: "image/png" });
    fireEvent.paste(composer, { clipboardData: { files: [image] } });
    expect(await screen.findByRole("alert")).toHaveTextContent("requires a folder-scoped project sandbox");
    expect(screen.queryByRole("img", { name: "clipboard.png" })).not.toBeInTheDocument();
  });
});
