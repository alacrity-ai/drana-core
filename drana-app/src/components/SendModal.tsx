import { useState } from 'react';
import { useWallet } from '../wallet/WalletContext';
import { usePending } from '../wallet/PendingContext';
import { getAccountByName } from '../api/nodeRpc';
import { Modal } from './Modal';

export function SendModal({ onClose }: { onClose: () => void }) {
  const { signAndSubmit, balance } = useWallet();
  const { addPending, pendingOutgoing } = usePending();
  const [to, setTo] = useState('');
  const [resolvedAddr, setResolvedAddr] = useState('');
  const [amount, setAmount] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState('');

  const resolveName = async () => {
    if (to.startsWith('drana1')) { setResolvedAddr(to); return to; }
    try {
      const acct = await getAccountByName(to.trim());
      setResolvedAddr(acct.address);
      return acct.address;
    } catch { setError(`Name "${to}" not found`); return null; }
  };

  const handleSend = async () => {
    const microdrana = Math.floor(parseFloat(amount) * 1_000_000);
    if (isNaN(microdrana) || microdrana <= 0) { setError('Enter a valid amount'); return; }
    if (!to.trim()) { setError('Enter a recipient'); return; }
    const available = balance - pendingOutgoing;
    if (microdrana > available) { setError(`Insufficient balance. Available: ${(available / 1_000_000).toFixed(2)} DRANA`); return; }
    setLoading(true); setError('');
    const addr = await resolveName();
    if (!addr) { setLoading(false); return; }
    try {
      const resp = await signAndSubmit({ type: 1, recipient: addr, amount: microdrana });
      if (resp.accepted) {
        addPending({ hash: resp.txHash || '', type: 1, amount: microdrana });
        setSuccess(`Sent! Tx: ${resp.txHash?.slice(0, 16)}...`);
      }
      else setError(resp.error || 'Rejected');
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  if (success) return (
    <Modal title="Send DRANA" onClose={onClose}>
      <p style={{ color: 'var(--success)', marginBottom: 16 }}>{success}</p>
      <button className="btn-primary" onClick={onClose}>Close</button>
    </Modal>
  );

  return (
    <Modal title="Send DRANA" onClose={onClose}>
      <input placeholder="Recipient (drana1... or name)" value={to}
        onChange={e => { setTo(e.target.value); setResolvedAddr(''); }}
        onBlur={() => { if (to && !to.startsWith('drana1')) resolveName(); }}
        style={{ marginBottom: 4 }} />
      {resolvedAddr && resolvedAddr !== to && (
        <p className="mono" style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 8, wordBreak: 'break-all' }}>
          → {resolvedAddr}
        </p>
      )}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4, marginTop: 8 }}>
        <input type="number" step="0.1" min="0" value={amount} onChange={e => setAmount(e.target.value)} style={{ width: 120 }} />
        <span className="mono amber" style={{ fontSize: 14 }}>DRANA</span>
      </div>
      <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 16 }}>
        Balance: {(balance / 1_000_000).toFixed(2)} DRANA
      </p>
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <button className="btn-primary" onClick={handleSend} disabled={loading}>
        {loading ? 'Sending...' : 'Send'}
      </button>
    </Modal>
  );
}
