import type { EncryptedKey } from './crypto';

const WALLETS_KEY = 'drana_wallets';
const ACTIVE_KEY = 'drana_active_wallet';
const LEGACY_KEY = 'drana_wallet';

export type StoredWallet = {
  address: string;
  publicKey: string;
  encryptedKey: EncryptedKey;
  name: string;
};

// Migrate from single-wallet format if needed.
function migrate() {
  if (localStorage.getItem(WALLETS_KEY)) return; // already migrated
  const legacy = localStorage.getItem(LEGACY_KEY);
  if (!legacy) return;
  try {
    const wallet: StoredWallet = JSON.parse(legacy);
    localStorage.setItem(WALLETS_KEY, JSON.stringify([wallet]));
    localStorage.setItem(ACTIVE_KEY, wallet.address);
    localStorage.removeItem(LEGACY_KEY);
  } catch { /* corrupt, ignore */ }
}

export function loadAllWallets(): StoredWallet[] {
  migrate();
  const raw = localStorage.getItem(WALLETS_KEY);
  if (!raw) return [];
  try { return JSON.parse(raw); } catch { return []; }
}

export function loadActiveWallet(): StoredWallet | null {
  migrate();
  const addr = localStorage.getItem(ACTIVE_KEY);
  if (!addr) return null;
  const wallets = loadAllWallets();
  return wallets.find(w => w.address === addr) || wallets[0] || null;
}

export function setActiveWallet(address: string): void {
  localStorage.setItem(ACTIVE_KEY, address);
}

export function saveWallet(wallet: StoredWallet): void {
  const wallets = loadAllWallets();
  const idx = wallets.findIndex(w => w.address === wallet.address);
  if (idx >= 0) {
    wallets[idx] = wallet;
  } else {
    wallets.push(wallet);
  }
  localStorage.setItem(WALLETS_KEY, JSON.stringify(wallets));
  localStorage.setItem(ACTIVE_KEY, wallet.address);
}

export function removeWallet(address: string): void {
  let wallets = loadAllWallets();
  wallets = wallets.filter(w => w.address !== address);
  localStorage.setItem(WALLETS_KEY, JSON.stringify(wallets));
  const active = localStorage.getItem(ACTIVE_KEY);
  if (active === address) {
    if (wallets.length > 0) {
      localStorage.setItem(ACTIVE_KEY, wallets[0].address);
    } else {
      localStorage.removeItem(ACTIVE_KEY);
    }
  }
}

export function clearWallet(): void {
  localStorage.removeItem(WALLETS_KEY);
  localStorage.removeItem(ACTIVE_KEY);
}
