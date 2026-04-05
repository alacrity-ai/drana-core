import { useState } from 'react';
import { useWallet } from '../wallet/WalletContext';
import { usePending } from '../wallet/PendingContext';
import { Modal } from './Modal';
import type { RankedPost } from '../api/types';

export function BoostModal({ post, onClose }: { post: RankedPost; onClose: () => void }) {
  const { signAndSubmit, balance } = useWallet();
  const { addPending, pendingOutgoing } = usePending();
  const [amount, setAmount] = useState('0.5');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState('');

  const handleBoost = async () => {
    const microdrana = Math.floor(parseFloat(amount) * 1_000_000);
    if (isNaN(microdrana) || microdrana < 100_000) { setError('Minimum 0.1 DRANA'); return; }
    const available = balance - pendingOutgoing;
    if (microdrana > available) { setError(`Insufficient balance. Available: ${(available / 1_000_000).toFixed(2)} DRANA`); return; }
    setLoading(true); setError('');
    try {
      const resp = await signAndSubmit({ type: 3, postId: post.postId, amount: microdrana });
      if (resp.accepted) {
        addPending({ hash: resp.txHash || '', type: 3, amount: microdrana });
        setSuccess(`Boosted! Tx: ${resp.txHash?.slice(0, 16)}...`);
      }
      else setError(resp.error || 'Rejected');
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  if (success) return (
    <Modal title="Boost Post" onClose={onClose}>
      <p style={{ color: 'var(--success)', marginBottom: 16 }}>{success}</p>
      <button className="btn-primary" onClick={onClose}>Close</button>
    </Modal>
  );

  return (
    <Modal title="Boost Post" onClose={onClose}>
      <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 4 }}>
        "{post.text.length > 80 ? post.text.slice(0, 80) + '...' : post.text}"
      </p>
      <p style={{ color: 'var(--text-muted)', fontSize: 13, marginBottom: 16 }}>
        Currently {(post.totalStaked / 1_000_000).toFixed(2)} DRANA staked
      </p>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <input type="number" step="0.1" min="0.1" value={amount} onChange={e => setAmount(e.target.value)} style={{ width: 120 }} />
        <span className="mono amber" style={{ fontSize: 14 }}>DRANA</span>
      </div>
      {parseFloat(amount) > 0 && (
        <div style={{ fontSize: 12, marginBottom: 12, lineHeight: 1.6 }}>
          <div style={{ color: 'var(--error)' }}>Burned: {(parseFloat(amount) * 0.03).toFixed(2)} DRANA (3%)</div>
          <div style={{ color: 'var(--text-secondary)' }}>Author: {(parseFloat(amount) * 0.02).toFixed(2)} DRANA (2%)</div>
          <div style={{ color: 'var(--text-secondary)' }}>Stakers: {(parseFloat(amount) * 0.01).toFixed(2)} DRANA (1%)</div>
          <div style={{ color: 'var(--success)' }}>Your stake: {(parseFloat(amount) * 0.94).toFixed(2)} DRANA (recoverable)</div>
        </div>
      )}
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <button className="btn-primary" onClick={handleBoost} disabled={loading}>
        {loading ? 'Staking...' : 'Stake on Post'}
      </button>
      <p style={{ color: 'var(--text-secondary)', fontSize: 12, marginTop: 12 }}>
        You can unstake and recover {(parseFloat(amount || '0') * 0.94).toFixed(2)} DRANA at any time.
      </p>
    </Modal>
  );
}
