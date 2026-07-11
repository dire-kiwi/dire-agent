import type { FeatureDoc } from "./types";

export const agentFeatures: FeatureDoc[] = [
  {
    slug: "streaming-queues-abort",
    title: "Streaming, queues, and abort",
    group: "Agent controls",
    summary: "Watch streaming output, steer an active turn, queue follow-ups, and cancel work.",
    prerequisites: ["Open an online conversation with Luna selected."],
    steps: [
      { action: "Send `Count slowly from 1 to 40, one number per line.`", expected: "The run badge changes to Running and assistant text appears incrementally." },
      { action: "While it runs, choose Steer beside the composer and send `Stop counting at 12.`", expected: "A steering item is queued or injected and the queue badge reflects pending guidance." },
      { action: "Choose Follow-up and send `Now reply with FOLLOW_UP_OK.`", expected: "The follow-up waits until the active run settles, then starts as the next turn." },
      { action: "Start another long answer and click Abort.", expected: "The active request is canceled, the run returns to Ready, and the UI remains usable." },
    ],
  },
  {
    slug: "slash-commands",
    title: "Interactive slash commands",
    group: "Agent controls",
    summary: "Use the browser composer as an interactive command prompt without sending every command to the model.",
    prerequisites: ["Open any conversation."],
    steps: [
      { action: "Enter `/help`.", expected: "A local system message lists every supported browser command." },
      { action: "Enter `/status`, `/model`, `/thinking`, and `/name` one at a time.", expected: "Status is appended locally; the other commands show the current value in a notice without model calls." },
      { action: "Enter `/name Browser command test` and `/thinking low`.", expected: "The conversation is renamed and the reasoning setting updates." },
      { action: "Enter `/follow-up FOLLOW_COMMAND_OK` while idle.", expected: "The queued prompt starts and the assistant eventually answers `FOLLOW_COMMAND_OK` when instructed exactly." },
      { action: "Enter `/clear`, then switch away and return.", expected: "The current view clears locally, while reopening restores persisted server history." },
      { action: "Enter `/quit`, then use Connection to reconnect.", expected: "The browser client disconnects cleanly and can reconnect without page reload." },
    ],
  },
  {
    slug: "subagents",
    title: "Persistent subagents",
    group: "Agent controls",
    summary: "Spawn bounded child agents, inspect their tree and transcripts, message them, wait, and interrupt.",
    prerequisites: ["Subagents are enabled in Settings.", "Open a project if the child needs local read tools."],
    steps: [
      { action: "Open conversation details, expand Subagents, enter a task such as `Inspect README.md and summarize its purpose`, and spawn the `explore` profile.", expected: "A child appears in the agent tree with a running status and its own durable ID." },
      { action: "Select the child row.", expected: "Its detail view shows profile, status, inherited tool policy, and transcript/events." },
      { action: "Send the child `Focus on the security boundary section.`", expected: "The message is stored in the child mailbox and wakes it if idle." },
      { action: "Use Wait for the child.", expected: "The result reports completion or the bounded wait timeout; a completed child auto-reports to its parent." },
      { action: "Spawn a second long-running child and click Interrupt.", expected: "The child transitions out of running without affecting the parent conversation." },
    ],
  },
];
