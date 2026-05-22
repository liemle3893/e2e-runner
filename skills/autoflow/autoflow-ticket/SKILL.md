---
name: autoflow-ticket
description: "Create, analyze, enrich, or update Jira tickets. Supports multiple modes: create tickets from implementation plans, analyze existing tickets for quality, enrich tickets with technical AC, and update epic status. Triggers on: 'Analyze PROJ-123', 'Create tickets from plan X', 'Enrich PROJ-123', 'Update PROJ-123'. Works with the jira-ticket-agent for execution."
argument-hint: "<Analyze|Enrich|Create|Update> <TICKET-KEY | plan path>"
metadata:
  context: fork
  model: opus
---

> NOTE: This skill still uses Atlassian MCP for edit/create/link. Read-path migration to REST is in autoflow-deliver; full migration is tracked separately.

# Write Jira Ticket

$ARGUMENTS

---

## Modes

Detect mode from user input:

| Trigger Pattern | Mode | What Happens |
|-----------------|------|--------------|
| `Analyze PROJ-###` | `analyze` | Fetch ticket, codebase scan, quality report |
| `Enrich PROJ-###` | `enrich` | Add technical AC, failure modes, security, test strategy |
| `Create tickets from <plan>` / plan file path | `create-from-plan` | Decompose plan into epic/stories/subtasks with links |
| `Update PROJ-###` | `update` | Sync ticket description, status, or links |
| Free text description | `create-single` | Create one ticket from description |

If mode is ambiguous, ask the user.

---

## Setup

**Jira Connection:** Run `autoflow deliver jira config get --field cloudId`.
If the command exits non-zero (nothing cached): call `mcp__atlassian__getAccessibleAtlassianResources` MCP tool, then ask user for their Atlassian email and run:
```bash
autoflow deliver jira config set --cloud-id <cloudId> --site-url <siteUrl> --project-key <projectKey> --email <email>
```

**Important:** Always include `--email` — it is required by `autoflow deliver jira upload` and `... download` for REST API authentication.

---

## Core Principles

1. One ticket = one shippable outcome (usually one PR)
2. AC = observable outcomes. Max 7 — more means split.
3. Every ticket needs >= 2 unhappy paths (API/UI), >= 1 (others). Spikes exempt.
4. Effort > 3 days → split
5. PO writes WHAT/WHY — agent adds HOW
6. When in doubt, ask PO — don't guess

---

## Security Classification

| Level | Criteria | Review |
|-------|----------|--------|
| **Critical** | Auth, encryption, PII, payments, file uploads, user input in HTML | Security review + SAST |
| **Relevant** | API endpoints, DB queries, external calls, config | Security review recommended |
| **Neutral** | Internal tooling, docs, styling, non-PII logging | Standard review |

## AI Complexity

| Level | Indicators | Files |
|-------|-----------|-------|
| **Routine** | Config, CRUD, single-file, existing pattern | 1-2 |
| **Moderate** | 2-4 files, pattern adaptation | 2-5 |
| **Complex** | Cross-cutting, novel logic, edge-case heavy | 5+ |

---

## Mode: analyze

**Purpose:** Assess ticket quality and readiness for implementation.

1. Fetch ticket from Jira via `mcp__atlassian__getJiraIssue`
2. Spawn `jira-ticket-agent` (or use inline if simple):

```
Agent tool:
  prompt: |
    <files_to_read>
    - ./CLAUDE.md
    - ./.claude/skills/autoflow-ticket/references/templates.md
    </files_to_read>

    Mode: ANALYZE
    Ticket: {TICKET_KEY}
    Ticket content:
    ---
    {ticket description from Jira}
    ---

    Run the quality checklist from SKILL.md silently. Then produce a terse report.

    OUTPUT FORMAT (hard rules):
    - ≤ 250 words total. If you exceed 250, you are including passing items — stop.
    - Do NOT restate the ticket. Assume the reader has it open.
    - Do NOT echo passing checks individually. Collapse to "Score: N/M passed."
    - Only expand on FAILURES, with a one-line fix per failure.
    - Max 5 affected files, paths + 1-line reason only. No code snippets.

    REQUIRED STRUCTURE:
    Verdict: PASS | NEEDS-WORK | FAIL — <one sentence why>
    Score: N/M checks passed

    Failures (only the ones that failed):
    - [#<check>] <what's wrong> → <concrete fix>

    Affected files (max 5):
    - path — reason

    Checklist to run silently:
    1. AC count ≤ 7, all binary/testable, no vague language
    2. ≥ 2 unhappy paths for API/UI tickets
    3. Security classification assigned
    4. Out of Scope bounded
    5. Dependencies Jira-linked (not just mentioned in prose)
    6. Data storage verified (grep table/collection names)
    7. Within word budget per templates.md Length Rules
    8. No empty/"None"/"N/A" sections
    9. No AC ↔ Failure Modes duplication
```

3. Present report to user
4. Offer next actions: "Enrich it?", "Add subtasks?", "Looks good — add AI-Reviewed label?"

---

## Mode: enrich

**Purpose:** Add technical depth to an existing ticket.

1. Fetch ticket from Jira
2. If ticket has no `## Original Requirement` section, wrap existing description as original requirement
3. Spawn `jira-ticket-agent` with mode=enrich:
   - Reads CLAUDE.md for project conventions
   - Scans codebase for affected files, patterns, similar code
   - Adds sections following template from `references/templates.md`:
     - Context, Scope, Out of Scope
     - Behavioral AC (max 7, binary, testable, >= 2 unhappy paths)
     - Failure Modes (>= 3 rows for API tickets)
     - Security Classification
     - Constraints (max 5)
     - Test Strategy
     - AI Complexity Estimate
     - Key Files (verified from codebase)
     - Rollback Strategy (if state/schema changes)
4. Self-check: run quality checklist on enriched result
5. If over word budget, trim in this exact order (stop as soon as you're under):
   a. Delete any section that would say "None", "N/A", or restate a CLAUDE.md global rule
   b. Delete Failure Modes rows that duplicate AC bullets
   c. Collapse AI Complexity Estimate to a single "tricky areas" line
   d. Cut Constraints items that aren't ticket-specific
   e. Still over? STOP — the ticket is too big. Split it instead of compressing further.
6. If PASS: update ticket via `mcp__atlassian__editJiraIssue`, add `AI-Reviewed` label
7. If FAIL (non-length): fix the specific issue, re-check

---

## Mode: create-from-plan

**Purpose:** Decompose an implementation plan into properly structured Jira tickets.

### Step 1: Read & Parse Plan

Read the plan file. Extract: goal, architecture, tech stack, depends-on, file structure, tasks.

### Step 2: Determine Structure

- If plan references an existing Epic key: create Stories + Subtasks under it
- If no Epic exists: create Epic first, then Stories + Subtasks

**Mapping rules (adapt to plan size):**
- **Single-milestone plan:** Each plan file = one Story. Each task = one Subtask.
- **Multi-milestone plan:** Each milestone = one Story (with steps as Subtasks) if steps are small/sequential (e.g., migrations, config). Each step = one Story if steps are independently shippable features with their own routes, tests, and verification.
- **Judgment call:** If a step has its own repository + service + handler + tests, it's a Story. If it's a config change or migration, it's a Subtask.

### Step 3: Create Epic (if needed)

Use `mcp__atlassian__createJiraIssue` with type `Epic`:
- Summary: `[Feature] <system name>`
- Description: overview, scope, out of scope, implementation plans table, security, complexity
- Labels: feature-specific label

### Step 4: Create Stories

For each plan, create a Story under the Epic:
- Summary: `[BE]` or `[FE]` prefix + plan goal (< 70 chars)
- Description follows Feature template from `references/templates.md`
- Behavioral AC from plan (max 7 per ticket)
- Failure modes, security, constraints from plan context
- Key files from plan's file structure
- Plan file reference
- Set `parent` to Epic key

### Step 5: Create Subtasks

For each task in a plan, create a Subtask under the Story:
- Summary: `[BE]` or `[FE]` + task title (< 70 chars)
- AC = the plan task's verification steps
- Key files = the plan task's files
- Security classification
- Set `parent` to Story key

### Step 5b: Upload Plan File

**This step is mandatory after the Epic is created.**

Upload the original plan file as an attachment to the Epic (and optionally to the first Story):
```bash
autoflow deliver jira upload <EPIC_KEY> <plan-file-path>
```

This ensures the full plan is always accessible from Jira, rather than embedding large file contents in ticket descriptions. In the `## Plan Files` section of ticket descriptions, reference the attachment:
```
Plan file attached: <filename>
```

### Step 6: Create Issue Links

**This step is mandatory after all tickets are created.**

Parse dependency chain from each plan's `Depends on:` line:
- Story A blocks Story B if Plan A must complete before Plan B
- Use `mcp__atlassian__createIssueLink` with type `Blocks`
- `inwardIssue` = the blocker, `outwardIssue` = the blocked

**Important:** Do NOT reference specific ticket keys (e.g., "Blocks: PROJ-35") in description text before those tickets exist. Use generic references like "Blocked by: M0 Foundation Story" in descriptions. The actual Jira issue links carry the dependency information — description text is supplementary.

### Step 7: Update Epic

**This step is mandatory after all tickets and links are created.**

Re-fetch Epic via `mcp__atlassian__getJiraIssue`. Update description:
- Add/update Implementation Plans table with: Plan name, Story key, Status
- Use `mcp__atlassian__editJiraIssue`

### Step 8: Report

Output a summary table of all created tickets with keys and links.

---

## Mode: update

**Purpose:** Update ticket description, status, or links.

1. Fetch ticket from Jira via `mcp__atlassian__getJiraIssue`
2. Determine what to update based on user's request:
   - "Update status" → use `mcp__atlassian__transitionJiraIssue`
   - "Update description" → use `mcp__atlassian__editJiraIssue`
   - "Add link to PROJ-###" → use `mcp__atlassian__createIssueLink`
   - "Update epic" → re-generate implementation plans table in Epic description
3. Execute update
4. Confirm to user

---

## Mode: create-single

**Purpose:** Create one ticket from a text description.

1. Parse user's description for: type (feature/bug/spike/tech-debt), scope, requirements
2. Choose template from `references/templates.md`
3. Scan codebase for affected files
4. Create ticket via `mcp__atlassian__createJiraIssue`
5. Run quality checklist
6. If PASS: add `AI-Reviewed` label

---

## Post-Action: Always

After ANY mode completes:

1. **Report created/modified tickets** — list all ticket keys with clickable links
2. **Check for missing links** — if tickets reference each other in text but aren't Jira-linked, create the links
3. **Check Epic status** — if tickets belong to an Epic, offer to update Epic description

---

## Quality Checklist [HARD GATE — blocks AI-Reviewed]

**If ANY fails → NO label, report issues, STOP.**

1. Implementable without asking PO? (Original content preserved verbatim.)
2. AC ≤ 7, binary and testable, concrete values — no "properly"/"appropriate".
3. ≥ 2 unhappy paths for API/UI. Failure Modes table covers validation + auth + downstream.
4. Scope bounded with Out of Scope. Effort ≤ 3 days or split.
5. Security classification assigned. Hot path → performance AC.
6. Dependencies Jira-linked (not just named in prose).
7. Data storage verified from codebase — not assumed.
8. Within word budget per templates.md Length Rules. No empty/"None"/"N/A" sections.
9. No duplication: AC ↔ Failure Modes, Constraints ↔ CLAUDE.md global rules.
10. **Bug tickets:** when the bug touches stateful side effects (data, counters, charges, notifications, exposure), the description MUST contain a `## Follow-ups` section. See `references/templates.md` Bug Report section for the trigger list and the section template.

---

## Templates

Read: `.claude/skills/autoflow-ticket/references/templates.md`

Feature, BE Subtask, FE Subtask, Bug Report, Spike, Tech Debt templates are defined there.

---

## Board Labels

| Label | Meaning | Who Sets |
|-------|---------|----------|
| `AI-Reviewed` | Passed quality checklist | This skill |
| `human-reviewed` | Human approved after AI review | Human |
| `AI-implemented` | Implementation agent completed | Implementation agent |
| `needs-discussion` | Bounced 3+ times | This skill |

Never remove labels — they serve as audit trail.

---

## Ticket Linking

| Type | Impact |
|------|--------|
| **Blocks / Blocked by** | Hard dependency — don't start blocked ticket |
| **Related to** | Proceed with awareness |

Use `mcp__atlassian__getIssueLinkTypes` to discover available link types if needed.
