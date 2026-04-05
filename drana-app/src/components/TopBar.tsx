import { useState } from 'react';
import { useWallet } from '../wallet/WalletContext';
import { usePending } from '../wallet/PendingContext';
import { CreateWalletModal, UnlockWalletModal, ImportWalletModal, RegisterNameModal } from '../wallet/WalletModals';
import { SendModal } from './SendModal';
import { DranaAmount } from './DranaAmount';
import { navigate } from '../router';

type ModalType = 'create' | 'unlock' | 'import' | 'register-name' | 'send' | null;

export function TopBar() {
  const wallet = useWallet();
  const { pendingTxs, pendingOutgoing } = usePending();
  const [modal, setModal] = useState<ModalType>(null);
  const [dropdownOpen, setDropdownOpen] = useState(false);

  const nameDisplay = wallet.name || (wallet.address ? wallet.address.slice(0, 10) + '...' : '');
  const activePending = pendingTxs.filter(t => t.status === 'pending');
  const hasPending = activePending.length > 0;
  const estimatedBalance = Math.max(0, wallet.balance - pendingOutgoing);

  const closeDropdown = () => setDropdownOpen(false);

  // What the wallet pill shows
  let pillLabel: string;
  let pillClick: () => void;
  if (!wallet.hasWallet) {
    pillLabel = 'Connect';
    pillClick = () => setDropdownOpen(!dropdownOpen);
  } else if (!wallet.isUnlocked) {
    pillLabel = nameDisplay;
    pillClick = () => setDropdownOpen(!dropdownOpen);
  } else {
    pillLabel = nameDisplay;
    pillClick = () => setDropdownOpen(!dropdownOpen);
  }

  return (
    <>
      <header className="topbar">
        <div className="topbar-inner">
          <div style={{ display: 'flex', alignItems: 'center', gap: 32 }}>
            <span className="logo" onClick={() => navigate('')}>DRANA</span>
            <nav style={{ display: 'flex', gap: 20 }}>
              <a href="#/" className="nav-link">Feed</a>
              <a href="#/channels" className="nav-link">Channels</a>
              {wallet.isUnlocked && <a href="#/rewards" className="nav-link">Rewards</a>}
            </nav>
          </div>
          <div style={{ position: 'relative' }}>
            <button className="wallet-pill" onClick={pillClick}>
              <span>{pillLabel}</span>
              {wallet.isUnlocked && (
                <span className="mono" style={{ marginLeft: 8 }}>
                  {hasPending && <span style={{ opacity: 0.7 }}>~</span>}
                  <DranaAmount microdrana={hasPending ? estimatedBalance : wallet.balance} size={13} />
                  {wallet.postStakeBalance > 0 && (
                    <span style={{ fontSize: 11, color: 'var(--text-muted)', marginLeft: 6 }}>📌{(wallet.postStakeBalance / 1_000_000).toFixed(0)}</span>
                  )}
                  {hasPending && <span style={{ fontSize: 10, color: 'var(--text-muted)', marginLeft: 4 }}>({activePending.length})</span>}
                </span>
              )}
              <span style={{ marginLeft: 6, fontSize: 10, color: 'var(--text-muted)' }}>▼</span>
            </button>

            {dropdownOpen && (
              <div className="dropdown" onClick={closeDropdown}>
                {/* Pending transactions */}
                {hasPending && (
                  <>
                    <div className="dropdown-label">Pending ({activePending.length})</div>
                    {activePending.map(tx => (
                      <div key={tx.hash} className="dropdown-item" style={{ fontSize: 12, color: 'var(--text-muted)', cursor: 'default' }}>
                        ⏳ {tx.type === 2 ? 'Post' : tx.type === 3 ? 'Stake' : tx.type === 7 ? 'Unstake' : tx.type === 1 ? 'Transfer' : 'Tx'}
                        {tx.channel ? ` in #${tx.channel}` : ''}
                        <span className="mono amber" style={{ marginLeft: 8 }}>
                          {(tx.amount / 1_000_000).toFixed(1)}
                        </span>
                      </div>
                    ))}
                    <div className="dropdown-sep" />
                  </>
                )}

                {/* Unlocked actions */}
                {wallet.isUnlocked && (
                  <>
                    <div className="dropdown-item" onClick={() => setModal('send')}>Send DRANA</div>
                    <div className="dropdown-item" onClick={() => { window.location.hash = '#/rewards'; closeDropdown(); }}>My Stakes & Rewards</div>
                    <div className="dropdown-item" onClick={() => setModal('register-name')}>Register Name</div>
                    <div className="dropdown-item" onClick={() => { navigator.clipboard.writeText(wallet.address!); }}>Copy Address</div>
                    <div className="dropdown-item" onClick={wallet.lock}>Lock Wallet</div>
                    <div className="dropdown-sep" />
                  </>
                )}

                {/* Locked — unlock */}
                {wallet.hasWallet && !wallet.isUnlocked && (
                  <>
                    <div className="dropdown-item" onClick={() => setModal('unlock')}>Unlock Wallet</div>
                    <div className="dropdown-sep" />
                  </>
                )}

                {/* Switch wallet — show other wallets */}
                {wallet.allWallets.length > 1 && (
                  <>
                    <div className="dropdown-label">Switch wallet</div>
                    {wallet.allWallets.filter(w => w.address !== wallet.address).map(w => (
                      <div key={w.address} className="dropdown-item"
                        onClick={() => wallet.switchWallet(w.address)}>
                        {w.name || (w.address.slice(0, 10) + '...' + w.address.slice(-6))}
                      </div>
                    ))}
                    <div className="dropdown-sep" />
                  </>
                )}

                {/* Always available */}
                <div className="dropdown-item" onClick={() => setModal('create')}>Create New Wallet</div>
                <div className="dropdown-item" onClick={() => setModal('import')}>Import Wallet</div>
              </div>
            )}
          </div>
        </div>
      </header>

      {modal === 'create' && <CreateWalletModal onClose={() => setModal(null)} />}
      {modal === 'unlock' && <UnlockWalletModal onClose={() => setModal(null)}
        onSwitchToCreate={() => setModal('create')} onSwitchToImport={() => setModal('import')} />}
      {modal === 'import' && <ImportWalletModal onClose={() => setModal(null)} />}
      {modal === 'register-name' && <RegisterNameModal onClose={() => setModal(null)} />}
      {modal === 'send' && <SendModal onClose={() => setModal(null)} />}

      <style>{`
        .topbar {
          height: var(--topbar-height); border-bottom: 1px solid var(--border);
          background: var(--bg-primary); position: sticky; top: 0; z-index: 50;
        }
        .topbar-inner {
          max-width: var(--max-width); margin: 0 auto; height: 100%;
          display: flex; align-items: center; justify-content: space-between;
          padding: 0 12px;
        }
        .logo {
          font-weight: 600; font-size: 18px; color: var(--accent);
          letter-spacing: 0.1em; cursor: pointer;
        }
        .nav-link { color: var(--text-secondary); font-size: 14px; font-weight: 500; }
        .nav-link:hover { color: var(--text-primary); }
        .wallet-pill {
          background: var(--bg-elevated); border: 1px solid var(--border);
          border-radius: 999px; padding: 6px 16px; font-size: 13px;
          color: var(--accent); font-weight: 500;
          display: flex; align-items: center;
        }
        .wallet-pill:hover { border-color: var(--accent); }
        .dropdown {
          position: absolute; right: 0; top: calc(100% + 8px);
          background: var(--bg-elevated); border: 1px solid var(--border);
          min-width: 200px; z-index: 60;
        }
        .dropdown-item {
          padding: 10px 16px; font-size: 13px; cursor: pointer;
          color: var(--text-secondary);
        }
        .dropdown-item:hover { background: var(--bg-surface); color: var(--text-primary); }
        .dropdown-sep { height: 1px; background: var(--border); margin: 4px 0; }
        .dropdown-label {
          padding: 6px 16px; font-size: 11px; font-weight: 500;
          text-transform: uppercase; letter-spacing: 0.05em;
          color: var(--text-muted);
        }
      `}</style>
    </>
  );
}
