import { useState } from 'react';
import { useWallet } from '../wallet/WalletContext';
import { usePending } from '../wallet/PendingContext';
import { Modal } from './Modal';

export function NewPostModal({ onClose, parentPostId }: { onClose: () => void; parentPostId?: string }) {
  const { signAndSubmit, balance } = useWallet();
  const { addPending, pendingOutgoing } = usePending();
  const [text, setText] = useState('');
  const [channel, setChannel] = useState('');
  const [amount, setAmount] = useState('1.0');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState('');

  const isReply = !!parentPostId;
  const title = isReply ? 'Reply' : 'New Post';

  const handleSubmit = async () => {
    const microdrana = Math.floor(parseFloat(amount) * 1_000_000);
    if (isNaN(microdrana) || microdrana < 1_000_000) { setError('Minimum 1.0 DRANA'); return; }
    if (!text.trim()) { setError('Text is required'); return; }
    const available = balance - pendingOutgoing;
    if (microdrana > available) { setError(`Insufficient balance. Available: ${(available / 1_000_000).toFixed(2)} DRANA`); return; }
    setLoading(true); setError('');
    try {
      const resp = await signAndSubmit({
        type: 2, // TxCreatePost
        text: text.trim(),
        channel: isReply ? '' : channel,
        postId: parentPostId,
        amount: microdrana,
      });
      if (resp.accepted) {
        addPending({ hash: resp.txHash || '', type: 2, text: text.trim(), channel: isReply ? '' : channel, amount: microdrana });
        setSuccess(`Posted! Tx: ${resp.txHash?.slice(0, 16)}...`);
      }
      else { setError(resp.error || 'Rejected'); }
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  if (success) return (
    <Modal title={title} onClose={onClose}>
      <p style={{ color: 'var(--success)', marginBottom: 16 }}>{success}</p>
      <button className="btn-primary" onClick={onClose}>Close</button>
    </Modal>
  );

  return (
    <Modal title={title} onClose={onClose}>
      {!isReply && (
        <input placeholder="Channel (optional, e.g. gaming)" value={channel}
          onChange={e => setChannel(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
          style={{ marginBottom: 8 }} />
      )}
      <textarea placeholder="Your message..." value={text} onChange={e => setText(e.target.value)}
        maxLength={280} rows={4} style={{ marginBottom: 4, resize: 'vertical' }} />
      <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 12, textAlign: 'right' }}>
        {text.length} / 280
      </p>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <input type="number" step="0.1" min="1" value={amount} onChange={e => setAmount(e.target.value)}
          style={{ width: 120 }} />
        <span className="mono amber" style={{ fontSize: 14 }}>DRANA</span>
      </div>
      {parseFloat(amount) > 0 && (
        <div style={{ fontSize: 12, marginBottom: 12, lineHeight: 1.6 }}>
          <div style={{ color: 'var(--error)' }}>Fee (6%): {(parseFloat(amount) * 0.06).toFixed(2)} DRANA burned</div>
          <div style={{ color: 'var(--success)' }}>Staked: {(parseFloat(amount) * 0.94).toFixed(2)} DRANA (recoverable)</div>
        </div>
      )}
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <button className="btn-primary" onClick={handleSubmit} disabled={loading}>
        {loading ? 'Posting...' : 'Post'}
      </button>
      <p style={{ color: 'var(--text-secondary)', fontSize: 12, marginTop: 12 }}>
        You can unstake and recover {(parseFloat(amount || '0') * 0.94).toFixed(2)} DRANA at any time.
      </p>
    </Modal>
  );
}
