import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getPost, getPostReplies } from '../api/indexerApi';
import { getMyPostStakes } from '../api/nodeRpc';
import { useWallet } from '../wallet/WalletContext';
import { DranaAmount } from '../components/DranaAmount';
import { TimeAgo } from '../components/TimeAgo';
import { BoostModal } from '../components/BoostModal';
import { UnstakeModal } from '../components/UnstakeModal';
import { NewPostModal } from '../components/NewPostModal';
import { CreateWalletModal, UnlockWalletModal } from '../wallet/WalletModals';
import { navigate } from '../router';

export function PostDetail({ id }: { id: string }) {
  const wallet = useWallet();
  const resolveName = wallet.resolveName;
  const [modal, setModal] = useState<'boost' | 'unstake' | 'reply' | 'wallet-create' | 'wallet-unlock' | null>(null);

  const post = useQuery({ queryKey: ['post', id], queryFn: () => getPost(id), retry: false });
  const replies = useQuery({ queryKey: ['replies', id], queryFn: () => getPostReplies(id), retry: false });
  const myStakes = useQuery({
    queryKey: ['my-post-stakes', wallet.address],
    queryFn: () => wallet.address ? getMyPostStakes(wallet.address) : Promise.resolve({ stakes: [], totalCount: 0, totalStaked: 0 }),
    enabled: !!wallet.address && wallet.isUnlocked,
  });

  const requireWallet = (action: () => void) => {
    if (!wallet.hasWallet) { setModal('wallet-create'); return; }
    if (!wallet.isUnlocked) { setModal('wallet-unlock'); return; }
    action();
  };

  if (post.isLoading) return <div style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>Loading...</div>;
  if (!post.data) return <div style={{ color: 'var(--error)', padding: 40, textAlign: 'center' }}>Post not found</div>;

  const p = post.data;
  const myStake = myStakes.data?.stakes?.find(s => s.postId === id);

  return (
    <div>
      <button onClick={() => history.back()} style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
        ← Back
      </button>

      {p.withdrawn && (
        <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--error)', padding: 12, marginBottom: 16 }}>
          <p style={{ fontSize: 13, color: 'var(--error)' }}>⚠ WITHDRAWN — This post's author has unstaked. Content preserved for history.</p>
        </div>
      )}

      <div style={{ background: 'var(--bg-surface)', padding: 24, borderBottom: '1px solid var(--border)' }}>
        <div style={{ marginBottom: 8 }}>
          <span style={{ marginRight: 6 }}>📌</span>
          <DranaAmount microdrana={p.totalStaked} size={28} />
        </div>
        <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 16 }}>
          {((p.authorStaked || 0) / 1_000_000).toFixed(2)} by author · {((p.thirdPartyStaked || 0) / 1_000_000).toFixed(2)} by {p.uniqueBoosterCount || 0} others{p.totalBurned ? ` · ${(p.totalBurned / 1_000_000).toFixed(2)} burned in fees` : ''}
        </p>
        <div style={{ fontSize: 14, color: 'var(--text-secondary)', marginBottom: 12 }}>
          <span style={{ cursor: 'pointer', color: 'var(--text-primary)', fontWeight: 500 }}
            onClick={() => navigate(`profile/${p.author}`)}>
            {resolveName(p.author) || (p.author.length > 20 ? p.author.slice(0, 10) + '...' : p.author)}
          </span>
          {p.channel && <span style={{ marginLeft: 8, color: 'var(--channel)' }}>#{p.channel}</span>}
          <span style={{ marginLeft: 8 }}>Block {p.createdAtHeight}</span>
          <span style={{ marginLeft: 8 }}><TimeAgo timestamp={p.createdAtTime} /></span>
        </div>
        <p style={{ fontSize: 18, fontWeight: 500, lineHeight: 1.5, marginBottom: 16 }}>{p.text}</p>
        {!p.withdrawn && (
          <div style={{ display: 'flex', gap: 8 }}>
            {myStake ? (
              <button className="btn-primary" style={{ width: 'auto', padding: '8px 20px', background: 'var(--error)' }}
                onClick={() => requireWallet(() => setModal('unstake'))}>
                Unstake ({(myStake.amount / 1_000_000).toFixed(2)} DRANA)
              </button>
            ) : (
              <button className="btn-primary" style={{ width: 'auto', padding: '8px 20px' }}
                onClick={() => requireWallet(() => setModal('boost'))}>Stake on this post</button>
            )}
          </div>
        )}
      </div>

      {/* Replies */}
      <div style={{ marginTop: 24 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <span className="label">REPLIES ({replies.data?.totalCount || 0})</span>
          {!p.withdrawn && (
            <button style={{ color: 'var(--accent)', fontSize: 13, fontWeight: 500 }}
              onClick={() => requireWallet(() => setModal('reply'))}>Write a reply</button>
          )}
        </div>
        {replies.data?.replies?.map(r => (
          <div key={r.postId} style={{ padding: '12px 16px', borderLeft: '3px solid var(--reply)', borderBottom: '1px solid var(--border)', background: 'var(--bg-surface)' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
              <DranaAmount microdrana={r.totalStaked} size={14} />
              <TimeAgo timestamp={r.createdAtTime} />
            </div>
            <span style={{ fontSize: 13, fontWeight: 500, cursor: 'pointer' }}
              onClick={() => navigate(`profile/${r.author}`)}>
              {resolveName(r.author) || (r.author.length > 20 ? r.author.slice(0, 10) + '...' : r.author)}
            </span>
            <p style={{ fontSize: 14, marginTop: 4 }}>{r.text}</p>
          </div>
        ))}
      </div>

      {/* Staker info */}
      {p.stakerCount > 0 && (
        <div style={{ marginTop: 24 }}>
          <span className="label" style={{ display: 'block', marginBottom: 12 }}>
            {p.stakerCount} STAKER{p.stakerCount !== 1 ? 'S' : ''} · {(p.totalStaked / 1_000_000).toFixed(2)} DRANA staked
          </span>
          {myStake && (
            <div className="mono" style={{ fontSize: 13, color: 'var(--success)', padding: '4px 0' }}>
              Your stake: {(myStake.amount / 1_000_000).toFixed(2)} DRANA (since block {myStake.height})
            </div>
          )}
        </div>
      )}

      {modal === 'boost' && <BoostModal post={p} onClose={() => setModal(null)} />}
      {modal === 'unstake' && myStake && <UnstakeModal post={p} stakeAmount={myStake.amount} onClose={() => setModal(null)} />}
      {modal === 'reply' && <NewPostModal parentPostId={id} onClose={() => setModal(null)} />}
      {modal === 'wallet-create' && <CreateWalletModal onClose={() => setModal(null)} />}
      {modal === 'wallet-unlock' && <UnlockWalletModal onClose={() => setModal(null)} />}
    </div>
  );
}
