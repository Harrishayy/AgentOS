# Claude Code Instructions — Agent Sandbox Runtime (P5 Extended)

Read this file fully before doing anything. These are standing rules for every session.

---

## Startup ritual — do this EVERY session automatically

When a session starts, before responding to anything:

1. Run `git branch --show-current` — if not on `p5/viewer-v2`, switch immediately
2. Read `context.md` fully
3. Tell the user in 3 lines:
   - Current branch
   - Last task completed
   - Next task to work on

Do not ask what to do next until this ritual is done.
If context.md does not exist, tell the user immediately.

---

## Autonomous loop — how every task runs

### Step 1 — Plan (show to user)
Write a detailed plan covering:
- Exactly what files you will create or modify
- Key technical decisions and reasoning
- Edge cases you are designing for
- How the user can verify it works

End with: "Ready to build. Confirm with 'yes' or tell me what to change."

### Step 2 — Build (only after user says yes)
Build the task completely. Do not stop mid-task for questions unless
you hit a genuine blocker. If something is ambiguous, make a reasonable
choice and note it.

### Step 3 — Verify (pause and wait for user)
Show:
- Every file created or modified with a brief description
- Exact commands to test it
- What success looks like

Say: "Please test and confirm it works. I will then commit and update context.md."

### Step 4 — Commit and update context.md (only after user confirms)
1. `git add viewer/` — never `git add .`
2. `git commit -m "p5: [specific description]"`
3. `git push origin p5/viewer-v2`
4. Update context.md — mark task done, update next task, add session log entry
5. Say: "Done. Next task: [name]. Say 'go' when ready."

### Step 5 — Next task (after user says go)
Return to Step 1 with the next task.

---

## The two moments you ALWAYS pause

1. After showing the plan — wait for "yes"
2. After showing test instructions — wait for "it works"

Never commit unverified code. Never skip these checkpoints.

---

## Token management

After completing each task, check if the session is getting long.
If yes, tell the user: "Session is getting long. Type /compact now,
then say 'go' to continue."

After /compact: re-read context.md immediately before doing anything.

---

## Session rule — ONE task at a time

Never start the next task until the current one is committed and context.md updated.

---

## Branch rule

Always on `p5/viewer-v2`. Never commit to main or another person's branch.

---

## File boundaries

Write and edit ONLY inside:
- `viewer/` — your entire domain
- `CLAUDE.md` — only if project changes require it
- `context.md` — update after every completed task
- `.cursor/rules/` — Cursor rules file

Read (never edit) for context:
- `p2/daemon/` — kernel event format
- `p4/orchestrator/` — LLM event format
- `README.md` — project overview

Never edit outside `viewer/` without explicit user confirmation.

---

## Files that must NEVER be committed

- `context.md` — gitignored, local only
- `.env`, `.env.local`
- `viewer/server/node_modules/`
- `viewer/viewer-app/node_modules/`
- `viewer/viewer-app/dist/`

Never run `git add .` — always explicit paths.

---

## What was already built (DO NOT rebuild these)

All of the following exist in main and are already working:
- viewer/index.html — static demo prototype
- viewer/server/server.js — WebSocket relay server on port 8765
  - Sender/viewer handshake, 3s timeout default
  - Malformed JSON dropped without crashing
  - MOCK_EVENTS=1 flag for testing
  - Clean SIGINT/SIGTERM shutdown
- viewer/viewer-app/ — React dashboard (Vite)
  - Header, AgentTabs, StatsRow, LLMPanel, KernelPanel
  - Live WebSocket connection, dual event streams
  - Blocked connection alert with red flash animation
  - Auto-reconnect on disconnect

When starting, read context.md to understand the current state of the codebase.

---

## Project purpose

A Linux sandbox that stops prompt-injected AI agents from making unauthorized
network connections — enforced at the kernel level using eBPF.

Your piece: the process viewer — adding security analysis and workflow
visualisation on top of the existing working dashboard.

---

## Tech stack (viewer/ only)

- Server: Node.js + `ws` library, port 8765
- Frontend: React with Vite
- Styling: Plain CSS
- Language: JavaScript only

---

## Event schemas — DO NOT deviate

LLM events (P4 → server → LEFT PANEL):
```json
{ "agent": "demo-agent", "type": "tool_call", "ts": 1234.5,
  "data": { "tool": "fetch_url", "args": { "url": "https://..." } } }
```
Types: stdout | tool_call | stopped | crashed

Kernel events (P2 → server → RIGHT PANEL):
```json
{ "agent": "demo-agent", "type": "connect_blocked", "ts": 1234.5,
  "data": { "dst_ip": "1.2.3.4", "dst_port": 80,
            "hostname": "evil.com", "reason": "no policy match" } }
```
Types: connect_attempt | connect_allowed | connect_blocked

New event type (your server → browser — SecurityPanel):
```json
{ "agent": "demo-agent", "type": "security_analysis", "ts": 1234.5,
  "data": { "threatLevel": "high", "summary": "...",
            "concerns": ["..."], "recommendation": "..." } }
```

---

## Extended tasks — what to build next

### Task 1: Analyse main branch (ALWAYS first this session)
Before any code:
```
git fetch origin
git log origin/main --oneline -20
git diff p5/viewer-v2..origin/main -- viewer/
```
Report findings. Do not build until this is done.

### Task 2: Security analysis engine
File: viewer/server/analyser.js

What it does:
- Maintains a rolling buffer of the last 20 events in server.js
- Every 30 seconds, sends buffer to Claude API for analysis
- Uses the OpenAI-compatible client pointed at Cisco's proxy:
  base_url: https://llm-proxy.dev.outshift.ai/
  api_key: process.env.OPENAI_API_KEY
- Broadcasts result as security_analysis event to all viewers

Analysis prompt returns JSON only:
{ "threatLevel": "low|medium|high|critical",
  "summary": "one sentence",
  "concerns": ["..."],
  "recommendation": "one sentence" }

Integration: analyser.js exports startAnalyser(getRecentEvents, broadcast)
server.js calls startAnalyser on startup

### Task 3: Security analysis panel (UI)
File: viewer/viewer-app/src/components/SecurityPanel.jsx

Shows:
- Threat level badge: low=green, medium=amber, high=red, critical=flashing red
- Summary, concerns list, recommendation
- Timestamp of last analysis
- Full width, sits below split panels in App.jsx

### Task 4: Workflow graph (UI)
File: viewer/viewer-app/src/components/WorkflowGraph.jsx
Library: reactflow (npm install reactflow in viewer/viewer-app/)

Shows:
- Visual directed graph of agent session
- New "Workflow" tab alongside existing "Events" tab
- Node types: start (green), tool_call (blue), stdout (grey dot),
  connect_allowed (green diamond), connect_blocked (RED diamond),
  stopped (green end), crashed (red end)
- convertEventsToGraph(events) converts event arrays to nodes/edges
- Updates in real time

---

## Model

Use whatever model is available on the current plan.
Use /compact when conversation exceeds ~20 messages.
Use /clear between tasks after context.md is updated.
