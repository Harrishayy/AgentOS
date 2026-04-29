import { useEffect, useMemo, useRef, useState } from 'react';
import './App.css';
import Header from './components/Header.jsx';
import AgentTabs from './components/AgentTabs.jsx';
import StatsRow from './components/StatsRow.jsx';
import LLMPanel from './components/LLMPanel.jsx';
import KernelPanel from './components/KernelPanel.jsx';

const WS_URL = 'ws://localhost:8765';
const RECONNECT_DELAY_MS = 3000;
const MAX_EVENTS = 500;

const LLM_TYPES = new Set(['stdout', 'tool_call', 'stopped', 'crashed']);
const KERNEL_TYPES = new Set(['connect_attempt', 'connect_allowed', 'connect_blocked']);

export default function App() {
  const [wsStatus, setWsStatus] = useState('disconnected');
  const [llmEvents, setLlmEvents] = useState([]);
  const [kernelEvents, setKernelEvents] = useState([]);
  const [activeAgent, setActiveAgent] = useState(null);
  const [uptime, setUptime] = useState(0);

  const socketRef = useRef(null);
  const reconnectTimerRef = useRef(null);
  const cancelledRef = useRef(false);

  useEffect(() => {
    cancelledRef.current = false;

    const connect = () => {
      if (cancelledRef.current) return;
      const ws = new WebSocket(WS_URL);
      socketRef.current = ws;

      ws.onopen = () => {
        ws.send(JSON.stringify({ role: 'viewer' }));
        setWsStatus('connected');
      };

      ws.onmessage = (msg) => {
        let event;
        try {
          event = JSON.parse(msg.data);
        } catch {
          console.warn('viewer: dropped malformed message');
          return;
        }
        if (!event || typeof event.type !== 'string') return;

        if (LLM_TYPES.has(event.type)) {
          setLlmEvents((prev) => [...prev, event].slice(-MAX_EVENTS));
        } else if (KERNEL_TYPES.has(event.type)) {
          setKernelEvents((prev) => [...prev, event].slice(-MAX_EVENTS));
        } else {
          console.warn('viewer: unknown event type', event.type);
        }
      };

      ws.onerror = () => {
        // onclose will fire next; reconnect is scheduled there.
      };

      ws.onclose = () => {
        setWsStatus('disconnected');
        socketRef.current = null;
        if (cancelledRef.current) return;
        reconnectTimerRef.current = setTimeout(connect, RECONNECT_DELAY_MS);
      };
    };

    connect();

    return () => {
      cancelledRef.current = true;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (socketRef.current) {
        socketRef.current.onclose = null;
        socketRef.current.close();
        socketRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    const start = Date.now();
    const id = setInterval(() => {
      setUptime(Math.floor((Date.now() - start) / 1000));
    }, 1000);
    return () => clearInterval(id);
  }, []);

  const agents = useMemo(() => {
    const names = new Set();
    for (const e of llmEvents) names.add(e.agent);
    for (const e of kernelEvents) names.add(e.agent);
    return Array.from(names);
  }, [llmEvents, kernelEvents]);

  useEffect(() => {
    if (activeAgent === null && agents.length > 0) {
      setActiveAgent(agents[0]);
    }
  }, [agents, activeAgent]);

  const stats = useMemo(() => {
    let toolCalls = 0;
    for (const e of llmEvents) if (e.type === 'tool_call') toolCalls += 1;
    let allowed = 0;
    let blocked = 0;
    for (const e of kernelEvents) {
      if (e.type === 'connect_allowed') allowed += 1;
      else if (e.type === 'connect_blocked') blocked += 1;
    }
    return { toolCalls, allowed, blocked, uptime };
  }, [llmEvents, kernelEvents, uptime]);

  const filteredLlm = activeAgent
    ? llmEvents.filter((e) => e.agent === activeAgent)
    : llmEvents;
  const filteredKernel = activeAgent
    ? kernelEvents.filter((e) => e.agent === activeAgent)
    : kernelEvents;

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
