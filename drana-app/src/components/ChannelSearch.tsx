import { useState, useRef, useEffect } from 'react';
import type { ChannelInfo } from '../api/types';

const MAX_PILLS = 5;
const MAX_RESULTS = 5;

export function ChannelSearch({ channels, selected, onSelect }: {
  channels: ChannelInfo[];
  selected: string;
  onSelect: (channel: string) => void;
}) {
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Top channels by post count.
  const topChannels = channels.slice(0, MAX_PILLS);
  const topNames = new Set(topChannels.map(c => c.channel));

  // Selected channel not in top list — show it as an extra pill.
  const showSelectedPill = selected && !topNames.has(selected);

  // Search results: prefix match, sorted alphabetically, capped.
  const searchResults = query
    ? channels
        .filter(c => c.channel.startsWith(query.toLowerCase()))
        .sort((a, b) => a.channel.localeCompare(b.channel))
        .slice(0, MAX_RESULTS)
    : channels.slice(0, MAX_RESULTS); // show top channels when empty

  // Close on click outside.
  useEffect(() => {
    if (!searchOpen) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setSearchOpen(false);
        setQuery('');
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [searchOpen]);

  // Focus input when search opens.
  useEffect(() => {
    if (searchOpen && inputRef.current) inputRef.current.focus();
  }, [searchOpen]);

  const selectChannel = (ch: string) => {
    onSelect(ch);
    setSearchOpen(false);
    setQuery('');
  };

  return (
    <div style={{ display: 'flex', gap: 6, marginBottom: 16, alignItems: 'center', flexWrap: 'wrap' }}>
      <button className={`ch-pill ${!selected ? 'active' : ''}`} onClick={() => selectChannel('')}>All</button>

      {topChannels.map(c => (
        <button key={c.channel} className={`ch-pill ${selected === c.channel ? 'active' : ''}`}
          onClick={() => selectChannel(c.channel)}>
          {c.channel || 'general'}
        </button>
      ))}

      {showSelectedPill && (
        <button className="ch-pill active" onClick={() => selectChannel('')}>
          {selected} <span style={{ marginLeft: 4, opacity: 0.6 }}>✕</span>
        </button>
      )}

      <div ref={containerRef} style={{ position: 'relative' }}>
        {!searchOpen ? (
          <button className="ch-pill" onClick={() => setSearchOpen(true)}
            style={{ color: 'var(--text-muted)' }}>
            🔍 ...
          </button>
        ) : (
          <>
            <input ref={inputRef} value={query} onChange={e => setQuery(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
              placeholder="Search channels..."
              onKeyDown={e => {
                if (e.key === 'Escape') { setSearchOpen(false); setQuery(''); }
                if (e.key === 'Enter' && searchResults.length > 0) selectChannel(searchResults[0].channel);
              }}
              style={{ width: 160, padding: '6px 12px', fontSize: 13, fontFamily: 'var(--font-mono)', background: 'var(--bg-primary)', border: '1px solid var(--accent)' }} />
            {searchResults.length > 0 && (
              <div className="ch-dropdown">
                {searchResults.map(c => (
                  <div key={c.channel} className="ch-result" onClick={() => selectChannel(c.channel)}>
                    <span style={{ color: 'var(--channel)' }}>#{c.channel || 'general'}</span>
                    <span style={{ color: 'var(--text-muted)', fontSize: 12 }}>({c.postCount})</span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      <style>{`
        .ch-pill {
          font-size: 13px; font-weight: 500; padding: 6px 14px;
          background: var(--bg-elevated); color: var(--text-secondary);
          border: 1px solid var(--border); white-space: nowrap;
          transition: all var(--transition); cursor: pointer;
        }
        .ch-pill.active { background: var(--accent); color: var(--bg-primary); border-color: var(--accent); }
        .ch-pill:hover:not(.active) { border-color: var(--accent); color: var(--text-primary); }
        .ch-dropdown {
          position: absolute; top: calc(100% + 4px); left: 0; z-index: 40;
          background: var(--bg-elevated); border: 1px solid var(--border);
          min-width: 200px;
        }
        .ch-result {
          padding: 8px 12px; cursor: pointer; font-size: 13px;
          display: flex; justify-content: space-between; gap: 12px;
        }
        .ch-result:hover { background: var(--bg-surface); }
      `}</style>
    </div>
  );
}
