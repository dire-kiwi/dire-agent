import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
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

describe("capability commands", () => {
  afterEach(() => cleanup());
  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
    mockState.projects = [projectFixture];
    mockState.capabilityCommands = [{
      name: "release",
      description: "Prepare a release summary",
      source: "extension:release-tools",
    }];
  });

  async function openCommands() {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByLabelText("Message the agent");
    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    await within(drawer).findByRole("option", { name: "/release" });
    return { user, drawer };
  }

  it("executes an extension command and reports output plus a queued prompt", async () => {
    mockState.capabilityCommandResult = {
      output: "Release notes generated",
      prompt: "Review and publish the release notes",
    };
    const { user, drawer } = await openCommands();
    await user.type(within(drawer).getByLabelText("Arguments for /release"), "v2.0 --draft");
    await user.click(within(drawer).getByRole("button", { name: "Run /release" }));

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "execute_capability_command",
      conversation_id: projectFixture.id,
      project_id: projectFixture.id,
      command_name: "release",
      arguments: "v2.0 --draft",
    })));
    expect(await within(drawer).findByText("Release notes generated")).toBeInTheDocument();
    expect(within(drawer).getByText("Prompt queued for the agent.")).toBeInTheDocument();
  });

  it("renders command-declared errors without hiding their output", async () => {
    mockState.capabilityCommandResult = { output: "Missing release tag", is_error: true };
    const { user, drawer } = await openCommands();
    await user.click(within(drawer).getByRole("button", { name: "Run /release" }));
    expect(await within(drawer).findByText("Command failed")).toBeInTheDocument();
    expect(within(drawer).getByText("Missing release tag")).toBeInTheDocument();
  });
});
