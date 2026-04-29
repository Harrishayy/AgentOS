import { useEffect, useRef } from 'react';
import './Panel.css';
import './EventRow.css';

function formatTime(ts) {
  if (!ts && ts !== 0) return '—';
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString([], { hour12: false }) + '.' +
    String(d.getMilliseconds()).padStart(3, '0');
}

function renderContent(event) {
  const data = event.data || {};
  switch (event.type) {
    case 'stdout':
      return data.line ?? '';
    case 'tool_call': {
      const args = data.args ? JSON.stringify(data.args) : '';
      return `${data.tool || 'unknown_tool'}(${args})`;
    }
    case 'stopped':
      return `agent exited cleanly (exit_code=${data.exit_code ?? 0})`;
    case 'crashed':
      return `agent crashed (exit_code=${data.exit_code ?? '?'})`;
    default:
      return JSON.stringify(data);
  }
}

export default function LLMPanel({ events }) {
  const bottomRef = useRef(null);

  useEffect(() => {
    if (bottomRef.current) {
      bottomRef.current.scrollIntoView({ block: 'end' });
    }
  }, [events.length]);

  return (
    <section className="panel">
      <header className="panel__header">
        <span className="panel__title">LLM events</span>
        <span className="panel__count">{events.length}</span>
      </header>
      <div className="panel__feed">
        {events.length === 0 ? (
          <div className="panel__empty">waiting for LLM events…</div>
        ) : (
          events.map((event, i) => (
            <div key={i} className={`event-row type-${event.type}`}>
              <span className="event-row__time">{formatTime(event.ts)}</span>
              <span className="event-row__badge">{event.type}</span>
              <span className="event-row__content">{renderContent(event)}</span>
            </div>
          ))
        )}
        <div ref={bottomRef} />
      </div>
    </section>
  );
}
