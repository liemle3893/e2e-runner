# Jira Transition Discovery

Reference loaded on-demand by `autoflow-deliver/SKILL.md`.

## Protocol

Transition IDs are instance-specific and must never be hardcoded. Always
discover at runtime through the `autoflow deliver jira` REST CLI — no MCP tools.

- List transitions: `autoflow deliver jira transitions <KEY>` (prints JSON).
- Apply a transition by name (case-insensitive): `autoflow deliver jira transition <KEY> --name '<Name>'`.
- Apply by explicit id (bypass name lookup): `autoflow deliver jira transition <KEY> --id <ID>`.

The CLI resolves name → id internally; on no match it exits non-zero and
prints the available transition names to stderr so you can escalate.

## Transition Table

| From | Transition Name | To Status |
|------|----------------|-----------|
| To Do | start dev | In Development |
| In Development | Dev Done | In Code Review |
| QC | QC Done, Start UAT | Ready for UAT |
| Any | Done | Done |

## Usage in autoflow-deliver

### Step 2 — Start Development (Jira mode only)

After worktree creation, transition from "To Do" → "In Development":

```bash
autoflow deliver jira transition <KEY> --name 'Start Dev'
```

**Rules:**
- Only transition if current status is "To Do" (from task-brief.md STATUS field)
- Skip if already "In Development" or later (resumed workflow)

### Step 13 — Dev Done (Jira mode only)

After all gates pass and PR is ready, transition from "In Development" →
"In Code Review":

```bash
autoflow deliver jira transition <KEY> --name 'Dev Done'
```

**Rules:**
- Ticket should already be in "In Development" (set by Step 2)
- If enhance loop escalated (Step 9 failed), SKIP this transition — ticket stays in "In Development"

### Troubleshooting

If the transition fails with `no transition named "X" (available: ...)`,
your workflow state does not match expectations (e.g. ticket not in the
required From status). Inspect with `autoflow deliver jira transitions <KEY>`
and escalate to the user.
