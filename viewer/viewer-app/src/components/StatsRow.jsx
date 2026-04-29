import './StatsRow.css';

function formatUptime(seconds) {
  const s = Math.max(0, Math.floor(seconds || 0));
  const m = Math.floor(s / 60);
  const r = s % 60;
  if (m === 0) return `${r}s`;
  return `${m}m ${r.toString().padStart(2, '0')}s`;
}

export default function StatsRow({ stats }) {
  const { toolCalls = 0, allowed = 0, blocked = 0, uptime = 0 } = stats || {};
  const cards = [
    { label: 'tool calls', value: toolCalls, tone: 'neutral' },
    { label: 'connections allowed', value: allowed, tone: 'good' },
    { label: 'connections blocked', value: blocked, tone: 'bad' },
    { label: 'uptime', value: formatUptime(uptime), tone: 'neutral' },
  ];
  return (
    <div className="stats-row">
      {cards.map((c) => (
        <div key={c.label} className={`stats-row__card tone-${c.tone}`}>
          <div className="stats-row__value">{c.value}</div>
          <div className="stats-row__label">{c.label}</div>
        </div>
      ))}
    </div>
  );
}
