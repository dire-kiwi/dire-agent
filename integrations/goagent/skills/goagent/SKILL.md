---
name: goagent
description: Manage persistent GoAgent chats and folder-scoped agent projects through the local daemon.
---

# GoAgent desktop bridge

Use the `goagent_*` MCP tools to work with the user's local GoAgent daemon.

- List conversations before assuming an ID.
- Use `goagent_create_chat` for a pathless conversation. A standalone chat has no project folder and receives no local file or shell tools.
- Use `goagent_create_project` only when the user supplied or confirmed an absolute folder. Project tools remain constrained to that folder.
- Use `goagent_send_message` to continue a conversation. It waits for the agentic loop to settle unless `wait` is explicitly false.
- Use `goagent_spawn_agent` for a bounded independent subtask, then `goagent_list_agents`, `goagent_wait_agents`, and `goagent_send_agent_message` to coordinate the team. Child agents cannot gain permissions their parent lacks.
- Interrupt a stuck child with `goagent_interrupt_agent`; delete only an idle leaf with `goagent_delete_agent`.
- Inspect `goagent_get_state` for running status, queues, token/cache usage, context fill, skills, and capabilities.
- Never claim that this transfers the current Codex conversation. It operates a separate persistent GoAgent conversation through standard MCP.

The GoAgent daemon accesses the subscription endpoint directly with Codex CLI credentials. This plugin does not use Codex app-server.
