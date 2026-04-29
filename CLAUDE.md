# Claude Code Instructions — Agent Sandbox Runtime (P5)

Read this file carefully before doing anything. These are standing rules for every session.

---

## Branch rule — ALWAYS verify first

Before touching any file, run:
```
git branch --show-current
```

If you are NOT on `p5/viewer`, switch immediately:
```
git checkout p5/viewer
```

Never commit to main, never push to another person's branch.

---

## File boundaries — ONLY work in viewer/

You may read, create, and edit files ONLY inside:
- `viewer/` — your entire domain
- `CLAUDE.md` — update if project changes
- `.gitignore` — only to add viewer-related entries

You may READ (not edit) these for context:
- `p2/daemon/` or `daemon/` — to understand kernel event format
- `p4/orchestrator/` or `orchestrator/` — to understand LLM event format
- `README.md` — project overview

You must NEVER edit anything outside `viewer/` without explicit user confirmation.

---

## Files that must NEVER be committed

These are in .gitignore and must stay there forever:
- `context.md` — personal session notes
- `.env` and `.env.local` — API keys
- `viewer/server/node_modules/`
- `viewer/viewer-app/node_modules/`
- `viewer/viewer-app/dist/`

If you are about to stage or commit any of these, stop and tell the user.

---

## After completing each task

1. Stage only viewer/ files: `git add viewer/`
2. Commit with prefix: `git commit -m "p5: [what you built]"`
3. Push to correct branch: `git push origin p5/viewer`
4. Tell the user to update context.md with what was done

Never run `git add .` — always be explicit about what you stage.

---

## Project purpose

A Linux sandbox that stops prompt-injected AI agents from making unauthorized
network connections — enforced at the kernel level using eBPF.

Your piece: the process viewer — a Node.js WebSocket server + React dashboard
showing LLM events (left panel) and kernel events (right panel) in real time.

---

## Tech stack (viewer/ only)

- Server: Node.js + `ws` library, port 8765
- Frontend: React with Vite
- Styling: Plain CSS
- Language: JavaScript only

---

## Event schemas (DO NOT deviate)

LLM events from P4:
```json
{ "agent": "demo-agent", "type": "tool_call", "ts": 1234.5,
  "data": { "tool": "fetch_url", "args": { "url": "https://..." } } }
```
Types: stdout | tool_call | stopped | crashed

Kernel events from P2:
```json
{ "agent": "demo-agent", "type": "connect_blocked", "ts": 1234.5,
  "data": { "dst_ip": "1.2.3.4", "dst_port": 80,
            "hostname": "evil.com", "reason": "no policy match" } }
```
Types: connect_attempt | connect_allowed | connect_blocked

---

## Model

Always use claude-sonnet-4-6. Never use Opus unless explicitly asked.
Use /compact when conversation exceeds ~20 messages.
