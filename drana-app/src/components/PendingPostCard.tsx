import type { PendingTx } from '../wallet/PendingContext';
import { DranaAmount } from './DranaAmount';

export function PendingPostCard({ tx }: { tx: PendingTx }) {
  return (
    <div className="pending-card">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 6 }}>
        <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--accent)', opacity: 0.7 }}>⏳ PENDING</span>
        <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>just now</span>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
        <span style={{ fontSize: 14, fontWeight: 500 }}>you</span>
        {tx.channel && <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--channel)' }}>#{tx.channel}</span>}
      </div>
      <p style={{ fontSize: 15, fontWeight: 500, marginBottom: 10, lineHeight: 1.4, opacity: 0.8 }}>
        {tx.text}
      </p>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ opacity: 0.6 }}>
          <DranaAmount microdrana={tx.amount} size={14} />
          <span style={{ color: 'var(--text-muted)', fontSize: 12, marginLeft: 8 }}>committed</span>
        </div>
        <span style={{ fontSize: 12, color: 'var(--text-muted)', fontStyle: 'italic' }}>
          Waiting for next block...
        </span>
      </div>

      <style>{`
        .pending-card {
          padding: 16px 20px;
          border-bottom: 1px solid var(--border);
          border-left: 3px dashed var(--accent);
          background: var(--accent-dim);
          opacity: 0.85;
        }
      `}</style>
    </div>
  );
}
