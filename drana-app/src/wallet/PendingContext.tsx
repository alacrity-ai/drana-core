import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { getTxStatus, getMempoolPending } from '../api/nodeRpc';
import { useQueryClient } from '@tanstack/react-query';

export type PendingTx = {
  hash: string;
  type: number;
  text?: string;
  channel?: string;
  amount: number;
  submittedAt: number;
  status: 'pending' | 'confirmed' | 'failed';
};

type PendingContextType = {
  pendingTxs: PendingTx[];
  addPending: (tx: Omit<PendingTx, 'submittedAt' | 'status'>) => void;
  getPendingPosts: () => PendingTx[];
  pendingOutgoing: number;
};

const STORAGE_KEY = 'drana_pending_txs';
const PendingCtx = createContext<PendingContextType>(null!);
export const usePending = () => useContext(PendingCtx);

function loadPending(): PendingTx[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    return JSON.parse(raw);
  } catch { return []; }
}

function savePending(txs: PendingTx[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(txs));
}

export function PendingProvider({ children, walletAddress }: { children: ReactNode; walletAddress: string | null }) {
  const [txs, setTxs] = useState<PendingTx[]>(loadPending);
  const queryClient = useQueryClient();

  // Persist to localStorage.
  useEffect(() => { savePending(txs); }, [txs]);

  // Poll pending tx statuses every 5 seconds.
  useEffect(() => {
    const pending = txs.filter(t => t.status === 'pending');
    if (pending.length === 0) return;

    const poll = async () => {
      let changed = false;
      const updated = await Promise.all(txs.map(async (tx) => {
        if (tx.status !== 'pending') return tx;

        // Timeout: if pending > 5 minutes, mark as failed.
        if (Date.now() - tx.submittedAt > 5 * 60 * 1000) {
          changed = true;
          return { ...tx, status: 'failed' as const };
        }

        try {
          const status = await getTxStatus(tx.hash);
          if (status.status === 'confirmed') {
            changed = true;
            return { ...tx, status: 'confirmed' as const };
          }
          if (status.status === 'unknown' && Date.now() - tx.submittedAt > 15_000) {
            // Not in mempool, not confirmed, and it's been > 15 seconds — likely rejected.
            changed = true;
            return { ...tx, status: 'failed' as const };
          }
        } catch { /* unreachable */ }
        return tx;
      }));

      if (changed) {
        setTxs(updated);
        // Invalidate feed and balance queries so confirmed txs show up.
        queryClient.invalidateQueries({ queryKey: ['feed'] });
        queryClient.invalidateQueries({ queryKey: ['channels'] });
        queryClient.invalidateQueries({ queryKey: ['account'] });
      }
    };

    const id = setInterval(poll, 5000);
    poll(); // Run immediately.
    return () => clearInterval(id);
  }, [txs, queryClient]);

  // On mount with a wallet, reconcile stale pending txs against the mempool.
  useEffect(() => {
    if (!walletAddress) return;
    const pending = txs.filter(t => t.status === 'pending');
    if (pending.length === 0) return;

    getMempoolPending(walletAddress).then(resp => {
      const mempoolHashes = new Set(resp.transactions.map(t => t.hash));
      let changed = false;
      const updated = txs.map(tx => {
        if (tx.status !== 'pending') return tx;
        // If not in mempool, check if confirmed; if neither, mark failed.
        if (!mempoolHashes.has(tx.hash)) {
          getTxStatus(tx.hash).then(s => {
            if (s.status === 'confirmed') {
              setTxs(prev => prev.map(p => p.hash === tx.hash ? { ...p, status: 'confirmed' as const } : p));
              queryClient.invalidateQueries({ queryKey: ['feed'] });
            } else if (s.status === 'unknown') {
              setTxs(prev => prev.map(p => p.hash === tx.hash ? { ...p, status: 'failed' as const } : p));
            }
          }).catch(() => {});
          changed = true;
        }
        return tx;
      });
      if (changed) setTxs(updated);
    }).catch(() => {});
    // Only run on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [walletAddress]);

  // Clean up old confirmed/failed txs (older than 10 minutes).
  useEffect(() => {
    const id = setInterval(() => {
      setTxs(prev => prev.filter(tx =>
        tx.status === 'pending' || Date.now() - tx.submittedAt < 10 * 60 * 1000
      ));
    }, 30000);
    return () => clearInterval(id);
  }, []);

  const addPending = useCallback((tx: Omit<PendingTx, 'submittedAt' | 'status'>) => {
    setTxs(prev => [...prev, { ...tx, submittedAt: Date.now(), status: 'pending' }]);
  }, []);

  const getPendingPosts = useCallback(() => {
    return txs.filter(t => t.status === 'pending' && t.type === 2); // TxCreatePost = 2
  }, [txs]);

  const pendingOutgoing = txs
    .filter(t => t.status === 'pending')
    .reduce((sum, t) => sum + t.amount, 0);

  return (
    <PendingCtx.Provider value={{ pendingTxs: txs, addPending, getPendingPosts, pendingOutgoing }}>
      {children}
    </PendingCtx.Provider>
  );
}
