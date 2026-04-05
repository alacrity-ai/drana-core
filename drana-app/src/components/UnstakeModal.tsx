import { useState } from 'react';
import { useWallet } from '../wallet/WalletContext';
import { usePending } from '../wallet/PendingContext';
import { Modal } from './Modal';
import { DranaAmount } from './DranaAmount';
import type { RankedPost } from '../api/types';

export function UnstakeModal({ post, stakeAmount, onClose }: {
  post: RankedPost;
  stakeAmount: number;
  onClose: () => void;
}) {
  const { signAndSubmit, address } = useWallet();
  const { addPending } = usePending();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState('');

  const isAuthor = address === post.author;

  const handleUnstake = async () => {
    setLoading(true); setError('');
    try {
      const resp = await signAndSubmit({ type: 7, postId: post.postId, amount: 0 });
      if (resp.accepted) {
        addPending({ hash: resp.txHash || '', type: 7, amount: 0 });
        setSuccess(`Unstaked! ${(stakeAmount / 1_000_000).toFixed(2)} DRANA returned.`);
      } else setError(resp.error || 'Rejected');
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  if (success) return (
    <Modal title="Unstake from Post" onClose={onClose}>
      <p style={{ color: 'var(--success)', marginBottom: 16 }}>{success}</p>
      <button className="btn-primary" onClick={onClose}>Close</button>
    </Modal>
  );

  return (
    <Modal title="Unstake from Post" onClose={onClose}>
      <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 4 }}>
        "{post.text.length > 80 ? post.text.slice(0, 80) + '...' : post.text}"
      </p>
      <p style={{ color: 'var(--text-muted)', fontSize: 13, marginBottom: 16 }}>
        by {post.author.slice(0, 12)}... · #{post.channel || 'general'}
      </p>
      <div style={{ marginBottom: 16 }}>
        <span style={{ fontSize: 14 }}>Your stake: </span>
        <DranaAmount microdrana={stakeAmount} size={16} />
      </div>
      <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 16 }}>
        This will return {(stakeAmount / 1_000_000).toFixed(2)} DRANA to your wallet.
      </p>
      {isAuthor && (
        <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--error)', padding: 12, marginBottom: 16 }}>
          <p style={{ fontSize: 12, color: 'var(--error)', lineHeight: 1.5 }}>
            You are the author. Unstaking will <strong>withdraw</strong> this post and refund all other stakers.
          </p>
        </div>
      )}
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <div style={{ display: 'flex', gap: 8 }}>
        <button className="btn-primary" onClick={handleUnstake} disabled={loading}
          style={{ background: 'var(--error)' }}>
          {loading ? 'Unstaking...' : 'Unstake'}
        </button>
        <button className="btn-primary" onClick={onClose} style={{ background: 'var(--bg-elevated)', color: 'var(--text-primary)' }}>
          Cancel
        </button>
      </div>
    </Modal>
  );
}
