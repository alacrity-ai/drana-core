export function TimeAgo({ timestamp }: { timestamp: number }) {
  const seconds = Math.floor(Date.now() / 1000 - timestamp);
  let text: string;
  if (seconds < 60) text = `${seconds}s ago`;
  else if (seconds < 3600) text = `${Math.floor(seconds / 60)}m ago`;
  else if (seconds < 86400) text = `${Math.floor(seconds / 3600)}h ago`;
  else text = `${Math.floor(seconds / 86400)}d ago`;
  return <span style={{ color: 'var(--text-muted)', fontSize: 13 }}>{text}</span>;
}
