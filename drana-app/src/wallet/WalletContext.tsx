import { createContext, useContext, useState, useEffect, useCallback, useRef, type ReactNode } from 'react';
import { generateKeyPair, deriveAddress, encryptPrivateKey, decryptPrivateKey, toHex, fromHex, sign } from './crypto';
import { computeSignableBytes } from './signableBytes';
import { saveWallet, loadActiveWallet, loadAllWallets, setActiveWallet, removeWallet as removeStoredWallet, type StoredWallet } from './storage';
import { getAccount, submitTransaction } from '../api/nodeRpc';
import type { SubmitTxResponse } from '../api/types';

type WalletContextType = {
  address: string | null;
  publicKey: string | null;
  name: string | null;
  isUnlocked: boolean;
  hasWallet: boolean;
  balance: number;
  stakedBalance: number;
  postStakeBalance: number;
  allWallets: StoredWallet[];
  createWallet: (password: string) => Promise<string>;
  unlock: (password: string) => Promise<void>;
  lock: () => void;
  importWallet: (privateKeyHex: string, password: string) => Promise<void>;
  switchWallet: (address: string) => void;
  removeWallet: (address: string) => void;
  signAndSubmit: (tx: UnsignedTx) => Promise<SubmitTxResponse>;
  refreshBalance: () => Promise<void>;
  resolveName: (address: string) => string | null;
};

export type UnsignedTx = {
  type: number;
  recipient?: string;
  postId?: string;
  text?: string;
  channel?: string;
  amount: number;
};

const WalletCtx = createContext<WalletContextType>(null!);
export const useWallet = () => useContext(WalletCtx);

export function WalletProvider({ children }: { children: ReactNode }) {
  const [stored, setStored] = useState<StoredWallet | null>(loadActiveWallet);
  const [allWallets, setAllWallets] = useState<StoredWallet[]>(loadAllWallets);
  const [privateKey, setPrivateKey] = useState<Uint8Array | null>(null);
  const [balance, setBalance] = useState(0);
  const [stakedBalance, setStakedBalance] = useState(0);
  const [postStakeBalance, setPostStakeBalance] = useState(0);
  const [nameCache, setNameCache] = useState<Map<string, string>>(new Map());
  const pendingNameLookups = useRef<Set<string>>(new Set());

  const address = stored?.address ?? null;
  const publicKey = stored?.publicKey ?? null;
  const name = stored?.name ?? null;
  const isUnlocked = privateKey !== null;
  const hasWallet = stored !== null;

  const refreshBalance = useCallback(async () => {
    if (!address) return;
    try {
      const acct = await getAccount(address);
      setBalance(acct.balance);
      setStakedBalance(acct.stakedBalance);
      setPostStakeBalance(acct.postStakeBalance || 0);
      if (acct.name) setNameCache(prev => new Map(prev).set(address, acct.name!));
      if (acct.name && stored && acct.name !== stored.name) {
        const updated = { ...stored, name: acct.name };
        setStored(updated);
        saveWallet(updated);
        setAllWallets(loadAllWallets());
      }
    } catch { /* offline */ }
  }, [address, stored]);

  useEffect(() => { refreshBalance(); }, [refreshBalance]);
  useEffect(() => {
    if (!address) return;
    const id = setInterval(refreshBalance, 15000);
    return () => clearInterval(id);
  }, [address, refreshBalance]);

  useEffect(() => {
    const handler = () => setPrivateKey(null);
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, []);

  const reloadWallets = () => setAllWallets(loadAllWallets());

  const createWallet = async (password: string): Promise<string> => {
    const kp = generateKeyPair();
    const addr = deriveAddress(kp.publicKey);
    const encrypted = await encryptPrivateKey(kp.privateKey, password);
    const wallet: StoredWallet = { address: addr, publicKey: toHex(kp.publicKey), encryptedKey: encrypted, name: '' };
    saveWallet(wallet);
    setStored(wallet);
    setPrivateKey(kp.privateKey);
    reloadWallets();
    return toHex(kp.privateKey);
  };

  const unlock = async (password: string) => {
    if (!stored) throw new Error('No wallet');
    const key = await decryptPrivateKey(stored.encryptedKey, password);
    setPrivateKey(key);
    refreshBalance();
  };

  const lock = () => { setPrivateKey(null); };

  const importWallet = async (privateKeyHex: string, password: string) => {
    const key = fromHex(privateKeyHex);
    if (key.length !== 64) throw new Error('Private key must be 64 bytes (128 hex chars)');
    const pub = key.slice(32);
    const addr = deriveAddress(pub);
    const encrypted = await encryptPrivateKey(key, password);
    const wallet: StoredWallet = { address: addr, publicKey: toHex(pub), encryptedKey: encrypted, name: '' };
    saveWallet(wallet);
    setStored(wallet);
    setPrivateKey(key);
    reloadWallets();
    refreshBalance();
  };

  const switchWallet = (addr: string) => {
    setPrivateKey(null);
    setActiveWallet(addr);
    const wallet = loadAllWallets().find(w => w.address === addr) ?? null;
    setStored(wallet);
    setBalance(0);
    setStakedBalance(0);
  };

  const removeWalletFn = (addr: string) => {
    removeStoredWallet(addr);
    reloadWallets();
    if (addr === address) {
      setPrivateKey(null);
      const remaining = loadAllWallets();
      setStored(remaining[0] ?? null);
    }
  };

  const signAndSubmit = async (tx: UnsignedTx): Promise<SubmitTxResponse> => {
    if (!privateKey || !address || !publicKey) throw new Error('Wallet not unlocked');
    const acct = await getAccount(address);
    const fullTx = {
      type: tx.type, sender: address, recipient: tx.recipient || '', postId: tx.postId || '',
      text: tx.text || '', channel: tx.channel || '', amount: tx.amount, nonce: acct.nonce + 1, pubKey: publicKey,
    };
    const signable = computeSignableBytes(fullTx);
    const signature = sign(privateKey, signable);
    const resp = await submitTransaction({
      type: ['', 'transfer', 'create_post', 'boost_post', 'register_name', 'stake', 'unstake', 'unstake_post'][fullTx.type] || 'transfer',
      sender: fullTx.sender, recipient: fullTx.recipient || undefined, postId: fullTx.postId || undefined,
      text: fullTx.text || undefined, channel: fullTx.channel || undefined, amount: String(fullTx.amount),
      nonce: String(fullTx.nonce), signature: toHex(signature), pubKey: fullTx.pubKey,
    });
    if (resp.accepted) refreshBalance();
    return resp;
  };

  const resolveName = (addr: string): string | null => {
    if (nameCache.has(addr)) return nameCache.get(addr) || null;
    // Don't fetch the same address twice concurrently.
    if (pendingNameLookups.current.has(addr)) return null;
    pendingNameLookups.current.add(addr);
    getAccount(addr).then(acct => {
      pendingNameLookups.current.delete(addr);
      if (acct.name) {
        setNameCache(prev => new Map(prev).set(addr, acct.name!));
      }
    }).catch(() => { pendingNameLookups.current.delete(addr); });
    return null;
  };

  return (
    <WalletCtx.Provider value={{
      address, publicKey, name, isUnlocked, hasWallet, balance, stakedBalance, postStakeBalance, allWallets,
      createWallet, unlock, lock, importWallet, switchWallet, removeWallet: removeWalletFn,
      signAndSubmit, refreshBalance, resolveName,
    }}>
      {children}
    </WalletCtx.Provider>
  );
}
