import { useEffect, useRef } from 'react';
import './Panel.css';
import './EventRow.css';
import AlertBanner from './AlertBanner.jsx';

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

export default function LLMPanel({ events, alert, injectionTargets, onDismissAlert }) {
  const bottomRef = useRef(null);

  useEffect(() => {
    if (bottomRef.current) {
      bottomRef.current.scrollIntoView({ block: 'end' });
    }
  }, [events.length]);

  const targets = injectionTargets || null;

  return (
    <section className="panel">
      <header className="panel__header">
        <span className="panel__title">LLM events</span>
        <span className="panel__count">{events.length}</span>
      </header>
      {alert && (
        <AlertBanner
          key={alert.kernelId}
          hostname={alert.hostname}
          reason={alert.reason}
          onDismiss={onDismissAlert}
        />
      )}
      <div className="panel__feed">
        {events.length === 0 ? (
          <div className="panel__empty">waiting for LLM events…</div>
        ) : (
          events.map((event) => {
            const isTarget = targets && targets.has(event._id);
            const cls =
              `event-row type-${event.type}` +
              (isTarget ? ' is-injection-target' : '');
            return (
              <div key={event._id} className={cls}>
                <span className="event-row__time">{formatTime(event.ts)}</span>
                <span className="event-row__badge">{event.type}</span>
                <span className="event-row__content">{renderContent(event)}</span>
              </div>
            );
          })
        )}
        <div ref={bottomRef} />
      </div>
    </section>
  );
}
