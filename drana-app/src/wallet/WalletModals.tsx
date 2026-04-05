import { useState } from 'react';
import { useWallet } from './WalletContext';
import { Modal } from '../components/Modal';

export function CreateWalletModal({ onClose }: { onClose: () => void }) {
  const { createWallet, address } = useWallet();
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [privateKeyHex, setPrivateKeyHex] = useState('');
  const [copied, setCopied] = useState(false);
  const [confirmed, setConfirmed] = useState(false);

  const handleCreate = async () => {
    if (password.length < 6) { setError('Password must be at least 6 characters'); return; }
    if (password !== confirm) { setError('Passwords do not match'); return; }
    setLoading(true);
    try {
      const keyHex = await createWallet(password);
      setPrivateKeyHex(keyHex);
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(privateKeyHex).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  // After creation: show the private key backup screen
  if (privateKeyHex) {
    return (
      <Modal title="Back Up Your Private Key" onClose={confirmed ? onClose : () => {}}>
        <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--error)', padding: 12, marginBottom: 16 }}>
          <p style={{ fontSize: 13, color: 'var(--error)', fontWeight: 600, marginBottom: 8 }}>
            Save this key now. It will never be shown again.
          </p>
          <p style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.5 }}>
            This is the only way to recover your wallet if you lose access to this browser.
            Anyone with this key has full control of your funds.
          </p>
        </div>
        <p className="label" style={{ marginBottom: 6 }}>YOUR ADDRESS</p>
        <p className="mono" style={{ fontSize: 12, color: 'var(--text-secondary)', wordBreak: 'break-all', marginBottom: 16 }}>
          {address}
        </p>
        <p className="label" style={{ marginBottom: 6 }}>YOUR PRIVATE KEY</p>
        <div style={{ position: 'relative', marginBottom: 16 }}>
          <textarea readOnly value={privateKeyHex} rows={3}
            style={{ fontFamily: 'var(--font-mono)', fontSize: 11, wordBreak: 'break-all', resize: 'none', background: 'var(--bg-primary)' }} />
          <button onClick={handleCopy}
            style={{ position: 'absolute', top: 8, right: 8, fontSize: 11, color: 'var(--accent)', padding: '2px 8px', border: '1px solid var(--border)', background: 'var(--bg-surface)' }}>
            {copied ? 'Copied!' : 'Copy'}
          </button>
        </div>
        <label style={{ display: 'flex', alignItems: 'flex-start', gap: 8, marginBottom: 16, cursor: 'pointer' }}>
          <input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)}
            style={{ width: 'auto', marginTop: 2 }} />
          <span style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.4 }}>
            I have saved my private key in a secure location. I understand that if I lose it, my wallet cannot be recovered.
          </span>
        </label>
        <button className="btn-primary" onClick={onClose} disabled={!confirmed}>
          I've Saved My Key
        </button>
      </Modal>
    );
  }

  return (
    <Modal title="Create Wallet" onClose={onClose}>
      <p style={{ color: 'var(--text-secondary)', marginBottom: 16, fontSize: 14 }}>
        Choose a password to encrypt your private key.
      </p>
      <input type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)} style={{ marginBottom: 8 }} />
      <input type="password" placeholder="Confirm password" value={confirm} onChange={e => setConfirm(e.target.value)} style={{ marginBottom: 16 }} />
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <button className="btn-primary" onClick={handleCreate} disabled={loading}>
        {loading ? 'Creating...' : 'Create Wallet'}
      </button>
    </Modal>
  );
}

export function UnlockWalletModal({ onClose, onSwitchToCreate, onSwitchToImport }: {
  onClose: () => void;
  onSwitchToCreate?: () => void;
  onSwitchToImport?: () => void;
}) {
  const { unlock, address, allWallets, switchWallet, removeWallet } = useWallet();
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [showForgot, setShowForgot] = useState(false);
  const [showRemoveConfirm, setShowRemoveConfirm] = useState(false);
  const [removeInput, setRemoveInput] = useState('');

  const handleUnlock = async () => {
    setLoading(true);
    try { await unlock(password); onClose(); }
    catch { setError('Wrong password'); }
    setLoading(false);
  };

  const handleRemove = () => {
    if (removeInput !== address) return;
    removeWallet(address!);
    onClose();
  };

  return (
    <Modal title="Unlock Wallet" onClose={onClose}>
      <p className="mono" style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 16, wordBreak: 'break-all' }}>
        {address}
      </p>
      <input type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)}
        onKeyDown={e => e.key === 'Enter' && handleUnlock()} style={{ marginBottom: 8 }} />
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 8 }}>{error}</p>}
      <button className="btn-primary" onClick={handleUnlock} disabled={loading} style={{ marginBottom: 12 }}>
        {loading ? 'Unlocking...' : 'Unlock'}
      </button>

      {allWallets.length > 1 && (
        <div style={{ marginBottom: 12 }}>
          <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 6 }}>Switch to another wallet:</p>
          {allWallets.filter(w => w.address !== address).map(w => (
            <div key={w.address} style={{ padding: '6px 0', cursor: 'pointer', fontSize: 13, color: 'var(--accent)' }}
              onClick={() => { switchWallet(w.address); }}>
              {w.name || (w.address.slice(0, 10) + '...' + w.address.slice(-6))}
            </div>
          ))}
        </div>
      )}

      <button onClick={() => setShowForgot(!showForgot)}
        style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: showForgot ? 12 : 0 }}>
        Forgot password?
      </button>
      {showForgot && !showRemoveConfirm && (
        <div style={{ background: 'var(--bg-primary)', padding: 12, border: '1px solid var(--border)', marginBottom: 12 }}>
          <p style={{ fontSize: 12, color: 'var(--error)', marginBottom: 8, lineHeight: 1.5 }}>
            Your password cannot be recovered. Without it, this wallet's private key is inaccessible.
          </p>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {onSwitchToImport && (
              <button style={{ fontSize: 12, color: 'var(--accent)', padding: '4px 8px', border: '1px solid var(--border)' }}
                onClick={() => { onClose(); onSwitchToImport(); }}>Import from backup</button>
            )}
            {onSwitchToCreate && (
              <button style={{ fontSize: 12, color: 'var(--accent)', padding: '4px 8px', border: '1px solid var(--border)' }}
                onClick={() => { onClose(); onSwitchToCreate(); }}>Create new wallet</button>
            )}
            <button style={{ fontSize: 12, color: 'var(--error)', padding: '4px 8px', border: '1px solid var(--border)' }}
              onClick={() => setShowRemoveConfirm(true)}>Remove this wallet</button>
          </div>
        </div>
      )}
      {showRemoveConfirm && (
        <div style={{ background: 'var(--bg-primary)', padding: 12, border: '1px solid var(--error)' }}>
          <p style={{ fontSize: 12, color: 'var(--error)', marginBottom: 8, lineHeight: 1.5 }}>
            This will permanently remove this wallet. Type the full address to confirm:
          </p>
          <input placeholder="drana1..." value={removeInput} onChange={e => setRemoveInput(e.target.value)}
            style={{ fontSize: 11, fontFamily: 'var(--font-mono)', marginBottom: 8 }} />
          <button className="btn-primary" onClick={handleRemove} disabled={removeInput !== address}
            style={{ background: removeInput === address ? 'var(--error)' : undefined }}>
            Remove Wallet
          </button>
        </div>
      )}
    </Modal>
  );
}

export function ImportWalletModal({ onClose }: { onClose: () => void }) {
  const { importWallet } = useWallet();
  const [keyHex, setKeyHex] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleImport = async () => {
    if (password.length < 6) { setError('Password must be at least 6 characters'); return; }
    setLoading(true);
    try { await importWallet(keyHex.trim(), password); onClose(); }
    catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  return (
    <Modal title="Import Wallet" onClose={onClose}>
      <textarea placeholder="Private key (128 hex characters)" value={keyHex} onChange={e => setKeyHex(e.target.value)}
        rows={3} style={{ marginBottom: 8, fontFamily: 'var(--font-mono)', fontSize: 12 }} />
      <input type="password" placeholder="Password to encrypt" value={password} onChange={e => setPassword(e.target.value)} style={{ marginBottom: 16 }} />
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <button className="btn-primary" onClick={handleImport} disabled={loading}>
        {loading ? 'Importing...' : 'Import'}
      </button>
    </Modal>
  );
}

export function RegisterNameModal({ onClose }: { onClose: () => void }) {
  const { signAndSubmit, name: currentName, balance } = useWallet();
  const [nameInput, setNameInput] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState('');

  const validateName = (n: string): string | null => {
    if (n.length < 3) return 'At least 3 characters';
    if (n.length > 20) return 'At most 20 characters';
    if (!/^[a-z0-9_]+$/.test(n)) return 'Only a-z, 0-9, underscore';
    if (n.startsWith('_') || n.endsWith('_')) return 'Cannot start or end with underscore';
    if (n.includes('__')) return 'No consecutive underscores';
    return null;
  };

  const handleRegister = async () => {
    const cleaned = nameInput.toLowerCase().trim();
    const err = validateName(cleaned);
    if (err) { setError(err); return; }
    setLoading(true); setError('');
    try {
      const resp = await signAndSubmit({ type: 4, text: cleaned, amount: 0 });
      if (resp.accepted) setSuccess(`Name "${cleaned}" registered!`);
      else setError(resp.error || 'Rejected');
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  if (balance === 0) return (
    <Modal title="Register Name" onClose={onClose}>
      <p style={{ color: 'var(--text-secondary)', fontSize: 14 }}>
        You need DRANA in your wallet before registering a name. Get funds from another user or a faucet.
      </p>
      <button className="btn-primary" onClick={onClose} style={{ marginTop: 16 }}>Close</button>
    </Modal>
  );

  if (currentName) return (
    <Modal title="Register Name" onClose={onClose}>
      <p style={{ color: 'var(--text-secondary)', fontSize: 14 }}>
        Your name is <strong className="amber">"{currentName}"</strong>. Names are permanent.
      </p>
      <button className="btn-primary" onClick={onClose} style={{ marginTop: 16 }}>Close</button>
    </Modal>
  );

  if (success) return (
    <Modal title="Register Name" onClose={onClose}>
      <p style={{ color: 'var(--success)', marginBottom: 16 }}>{success}</p>
      <button className="btn-primary" onClick={onClose}>Close</button>
    </Modal>
  );

  return (
    <Modal title="Register Name" onClose={onClose}>
      <p style={{ color: 'var(--text-secondary)', marginBottom: 16, fontSize: 14 }}>
        Choose a permanent username. This cannot be changed.
      </p>
      <input placeholder="username" value={nameInput}
        onChange={e => setNameInput(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
        maxLength={20} style={{ marginBottom: 4, fontFamily: 'var(--font-mono)' }} />
      <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 16 }}>3-20 chars: a-z, 0-9, _</p>
      {error && <p style={{ color: 'var(--error)', fontSize: 13, marginBottom: 12 }}>{error}</p>}
      <button className="btn-primary" onClick={handleRegister} disabled={loading || nameInput.length < 3}>
        {loading ? 'Registering...' : 'Register'}
      </button>
    </Modal>
  );
}
