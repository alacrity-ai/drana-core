import { useQuery } from '@tanstack/react-query';
import { getChannels } from '../api/indexerApi';
import { navigate } from '../router';

export function Channels() {
  const { data, isLoading } = useQuery({ queryKey: ['channels'], queryFn: getChannels });

  if (isLoading) return <div style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>Loading...</div>;

  return (
    <div>
      <h1 style={{ fontSize: 18, fontWeight: 600, marginBottom: 16 }}>Channels</h1>
      {(!data || data.length === 0) && (
        <p style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>No channels yet. Create a post with a channel to start one.</p>
      )}
      {data?.map(c => (
        <div key={c.channel} onClick={() => { window.location.hash = `#/?channel=${c.channel}`; }}
          style={{ padding: '14px 20px', borderBottom: '1px solid var(--border)', background: 'var(--bg-surface)', cursor: 'pointer', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 15, fontWeight: 500, color: 'var(--channel)' }}>#{c.channel || 'general'}</span>
          <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{c.postCount} posts</span>
        </div>
      ))}
    </div>
  );
}
