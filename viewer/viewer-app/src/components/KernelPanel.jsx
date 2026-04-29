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
  const d = event.data || {};
  const target = d.hostname
    ? `${d.hostname} (${d.dst_ip}:${d.dst_port})`
    : `${d.dst_ip}:${d.dst_port}`;
  switch (event.type) {
    case 'connect_attempt':
      return `→ ${target}`;
    case 'connect_allowed':
      return `✓ ${target} — ${d.reason || 'allowed'}`;
    case 'connect_blocked':
      return `✗ ${target} — ${d.reason || 'blocked'}`;
    default:
      return JSON.stringify(d);
  }
}

export default function KernelPanel({ events }) {
  const bottomRef = useRef(null);

  useEffect(() => {
    if (bottomRef.current) {
      bottomRef.current.scrollIntoView({ block: 'end' });
    }
  }, [events.length]);

  return (
    <section className="panel">
      <header className="panel__header">
        <span className="panel__title">kernel events</span>
        <span className="panel__count">{events.length}</span>
      </header>
      <div className="panel__feed">
        {events.length === 0 ? (
          <div className="panel__empty">waiting for kernel events…</div>
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
