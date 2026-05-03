import { useEffect, useMemo } from 'react';
import ReactFlow, {
  Background, Controls, Handle, Position,
  ReactFlowProvider, useReactFlow,
} from 'reactflow';
import 'reactflow/dist/style.css';
import './WorkflowGraph.css';

// ─── Layout ──────────────────────────────────────────────────────────────────
const MAIN_X    = 60;
const KERNEL_X  = 370;
const NODE_GAP  = 130;
const START_Y   = 40;
const COL_WIDTH = 640;
const MATCH_WIN = 10; // seconds

// ─── Helpers ─────────────────────────────────────────────────────────────────
function trunc(str, n) {
  if (!str) return '';
  str = String(str);
  return str.length > n ? str.slice(0, n) + '…' : str;
}

function extractHostname(url) {
  try { return new URL(url).hostname; } catch { return trunc(url, 35); }
}

// ─── Custom node: main chain events ──────────────────────────────────────────
function EventNode({ data }) {
  return (
    <>
      <Handle type="target" position={Position.Top} />
      <div className={`wf-card wf-card--${data.variant}`}>
        <div className="wf-card__header">
          <span className="wf-card__icon">{data.icon}</span>
          <span className="wf-card__type">{data.typeLabel}</span>
          {data.step != null && (
            <span className="wf-card__step">#{data.step}</span>
          )}
        </div>
        {data.detail && (
          <div className="wf-card__detail">{data.detail}</div>
        )}
        {data.status && (
          <div className={`wf-card__status wf-card__status--${data.statusVariant}`}>
            {data.status}
          </div>
        )}
      </div>
      <Handle type="source" position={Position.Bottom} />
    </>
  );
}

// ─── Custom node: circle bookends (session_start / stopped / crashed) ─────────
function CircleNode({ data }) {
  return (
    <>
      {!data.isStart    && <Handle type="target" position={Position.Top} />}
      <div className={`wf-circle wf-circle--${data.variant}`}>
        <span className="wf-circle__icon">{data.icon}</span>
        <span className="wf-circle__label">{data.label}</span>
        {data.sub && <span className="wf-circle__sub">{data.sub}</span>}
      </div>
      {!data.isTerminal && <Handle type="source" position={Position.Bottom} />}
    </>
  );
}

// ─── Custom node: kernel outcome card ────────────────────────────────────────
function KernelNode({ data }) {
  const blocked = data.variant === 'blocked';
  return (
    <>
      <Handle type="target" position={Position.Top} />
      <div className={`wf-kernel wf-kernel--${data.variant}`}>
        <div className="wf-kernel__header">
          <span className="wf-kernel__icon">{blocked ? '⚠' : '✓'}</span>
          <span className="wf-kernel__type">
            {blocked ? 'INJECTION ATTEMPT BLOCKED' : 'CONNECTION ALLOWED'}
          </span>
        </div>
        <div className="wf-kernel__host">{data.host}</div>
        {data.reason && (
          <div className="wf-kernel__reason">Reason: {data.reason}</div>
        )}
      </div>
    </>
  );
}

const nodeTypes = { event: EventNode, circle: CircleNode, kernel: KernelNode };

// Smoothly re-fits the viewport whenever new nodes arrive.
function AutoFitView({ nodeCount }) {
  const { fitView } = useReactFlow();
  useEffect(() => {
    const t = setTimeout(
      () => fitView({ padding: 0.2, maxZoom: 1.1, duration: 350 }),
      60, // wait one frame for ReactFlow to measure new nodes
    );
    return () => clearTimeout(t);
  }, [nodeCount, fitView]);
  return null;
}

// ─── Pre-process: drop stdout, merge tool_call + tool_result ─────────────────
function preprocessLlm(events) {
  const filtered = events.filter((e) => e.type !== 'stdout');
  const result = [];
  let i = 0;
  while (i < filtered.length) {
    const e = filtered[i];
    if (e.type === 'tool_call') {
      const next = filtered[i + 1];
      if (next && next.type === 'tool_result' && next.data?.tool === e.data?.tool) {
        result.push({ ...e, _result: next.data });
        i += 2;
      } else {
        result.push({ ...e, _result: null });
        i += 1;
      }
    } else {
      result.push(e);
      i += 1;
    }
  }
  return result;
}

// ─── Node data builders ───────────────────────────────────────────────────────
const CIRCLE_TYPES = new Set(['session_start', 'stopped', 'crashed']);

function buildCircleData(event) {
  const d = event.data || {};
  switch (event.type) {
    case 'session_start':
      return {
        variant: 'start', isStart: true,
        icon: '▶', label: 'AGENT STARTED',
        sub: d.launch_mode ? `${event.agent} · ${d.launch_mode}` : (event.agent || ''),
      };
    case 'stopped':
      return {
        variant: 'complete', isTerminal: true,
        icon: '■', label: 'COMPLETE',
        sub: `exit code ${d.exit_code ?? 0}`,
      };
    case 'crashed':
      return {
        variant: 'crashed', isTerminal: true,
        icon: '✕', label: 'CRASHED',
        sub: `exit code ${d.exit_code ?? '?'}`,
      };
    default:
      return null;
  }
}

function buildEventData(event, step) {
  const d = event.data || {};
  switch (event.type) {
    case 'user_input':
      return {
        variant: 'user', icon: '👤',
        typeLabel: 'USER REQUEST',
        detail: trunc(d.text || '', 58),
        step,
      };

    case 'tool_call': {
      const url   = d.args?.url || '';
      const r     = event._result;
      const ready = r !== null && r !== undefined;
      const ok    = ready && r.ok !== false;
      return {
        variant:       ready ? (ok ? 'tool-ok' : 'tool-fail') : 'tool',
        icon:          '🌐',
        typeLabel:     'NETWORK REQUEST',
        detail:        url ? trunc(url, 50) : (d.tool || 'unknown'),
        status:        ready
                         ? (ok
                              ? `✓  ${r.status_code ?? ''} OK  ·  ${r.chars ?? '?'} chars`
                              : '✗  Request failed — connection refused or timed out')
                         : '⏳  awaiting result…',
        statusVariant: ready ? (ok ? 'ok' : 'fail') : 'pending',
        step,
      };
    }

    case 'agent_output':
      return {
        variant: 'agent', icon: '🤖',
        typeLabel: 'AGENT RESPONSE',
        detail: trunc(d.text || '', 58),
        step,
      };

    default:
      return {
        variant: 'default', icon: '◆',
        typeLabel: event.type.replace(/_/g, ' ').toUpperCase(),
        detail: trunc(JSON.stringify(d), 55),
        step,
      };
  }
}

// ─── Core conversion ──────────────────────────────────────────────────────────
function convertEventsToGraph(llmEvents, kernelEvents) {
  const kernel = kernelEvents.filter((e) => e.type !== 'connect_attempt');

  const agentLlm    = new Map();
  const agentKernel = new Map();
  for (const e of llmEvents) {
    const a = e.agent || 'unknown';
    if (!agentLlm.has(a)) agentLlm.set(a, []);
    agentLlm.get(a).push(e);
  }
  for (const e of kernel) {
    const a = e.agent || 'unknown';
    if (!agentKernel.has(a)) agentKernel.set(a, []);
    agentKernel.get(a).push(e);
  }

  const nodes = [];
  const edges = [];
  const allAgents = [...new Set([...agentLlm.keys(), ...agentKernel.keys()])];

  allAgents.forEach((agent, agentIdx) => {
    const colX    = agentIdx * COL_WIDTH;
    const mainX   = colX + MAIN_X;
    const kernelX = colX + KERNEL_X;

    const llmList    = preprocessLlm(agentLlm.get(agent) || []);
    const kernelList = agentKernel.get(agent) || [];

    let y      = START_Y;
    let prevId = null;
    let step   = 0;
    const toolCallNodes = []; // { id, ts, y }

    for (const event of llmList) {
      const id       = `n-${event._id}`;
      const isCircle = CIRCLE_TYPES.has(event.type);

      if (isCircle) {
        const cData = buildCircleData(event);
        // Center the 120px circle over the 240px card column
        nodes.push({ id, type: 'circle', position: { x: mainX + 60, y }, data: cData });
      } else {
        step += 1;
        const eData = buildEventData(event, step);
        nodes.push({ id, type: 'event', position: { x: mainX, y }, data: eData });
        if (event.type === 'tool_call') toolCallNodes.push({ id, ts: event.ts, y });
      }

      if (prevId) {
        edges.push({
          id: `e-${prevId}-${id}`,
          source: prevId, target: id,
          type: 'smoothstep',
          style: { stroke: '#2a3550', strokeWidth: 2 },
        });
      }
      prevId = id;
      y += isCircle ? 120 : NODE_GAP;
    }

    // Kernel nodes — linked to nearest preceding tool_call
    const usedTc = new Map();
    for (const ke of kernelList) {
      const kid     = `n-${ke._id}`;
      const variant = ke.type === 'connect_blocked' ? 'blocked' : 'allowed';
      const kd      = ke.data || {};
      const host    = kd.hostname || `${kd.dst_ip || '?'}:${kd.dst_port || '?'}`;

      let best = null;
      for (let i = toolCallNodes.length - 1; i >= 0; i--) {
        const tc = toolCallNodes[i];
        const dt = ke.ts - tc.ts;
        if (dt >= 0 && dt <= MATCH_WIN) { best = tc; break; }
      }

      const offset = best ? (usedTc.get(best.id) || 0) * 130 : 0;
      const nodeY  = best ? best.y + offset : y;
      if (best) usedTc.set(best.id, (usedTc.get(best.id) || 0) + 1);
      else y += 130;

      nodes.push({
        id: kid, type: 'kernel',
        position: { x: kernelX, y: nodeY },
        data: { variant, host: trunc(host, 28), reason: trunc(kd.reason || '', 40) },
      });

      const srcId = best ? best.id : prevId;
      if (srcId) {
        edges.push({
          id: `ek-${srcId}-${kid}`,
          source: srcId, target: kid,
          type: 'smoothstep',
          animated: variant === 'blocked',
          style: {
            stroke:           variant === 'blocked' ? '#ff4d5e' : '#3cd784',
            strokeWidth:      variant === 'blocked' ? 2.5 : 1.5,
            strokeDasharray:  variant === 'blocked' ? '6 3' : undefined,
          },
        });
      }
    }
  });

  return { nodes, edges };
}

// ─── Component ────────────────────────────────────────────────────────────────
export default function WorkflowGraph({ llmEvents, kernelEvents }) {
  const { nodes, edges } = useMemo(
    () => convertEventsToGraph(llmEvents, kernelEvents),
    [llmEvents, kernelEvents],
  );

  if (nodes.length === 0) {
    return (
      <div className="wf-wrapper">
        <div className="wf-empty">no events yet — switch to Events tab to see live data</div>
      </div>
    );
  }

  return (
    <div className="wf-wrapper">
      <ReactFlowProvider>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.2, maxZoom: 1.1 }}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
        >
          <AutoFitView nodeCount={nodes.length} />
          <Background color="#1c2536" gap={24} size={1} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}
