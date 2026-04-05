import { useQuery } from '@tanstack/react-query';
import { useWallet } from '../wallet/WalletContext';
import { getRewardSummary, getRewards, getRewardsForPost } from '../api/indexerApi';
import { getMyPostStakes } from '../api/nodeRpc';
import { DranaAmount } from '../components/DranaAmount';
import { TimeAgo } from '../components/TimeAgo';
import { navigate } from '../router';

export function Rewards() {
  const { address, isUnlocked, balance, postStakeBalance } = useWallet();

  const summary = useQuery({
    queryKey: ['reward-summary', address],
    queryFn: () => address ? getRewardSummary(address) : null,
    enabled: !!address && isUnlocked,
    retry: false,
  });

  const rewards = useQuery({
    queryKey: ['rewards', address],
    queryFn: () => address ? getRewards(address) : null,
    enabled: !!address && isUnlocked,
    retry: false,
  });

  const stakes = useQuery({
    queryKey: ['my-post-stakes', address],
    queryFn: () => address ? getMyPostStakes(address) : null,
    enabled: !!address && isUnlocked,
  });

  const rewardsPerPost = useQuery({
    queryKey: ['rewards-per-post', address, stakes.data?.stakes?.map(s => s.postId).join(',')],
    queryFn: async () => {
      if (!stakes.data?.stakes || !address) return {};
      const map: Record<string, number> = {};
      for (const s of stakes.data.stakes) {
        try {
          const r = await getRewardsForPost(address, s.postId);
          map[s.postId] = r.totalReward;
        } catch { map[s.postId] = 0; }
      }
      return map;
    },
    enabled: !!address && !!stakes.data?.stakes?.length,
    retry: false,
  });

  if (!isUnlocked) {
    return <div style={{ color: 'var(--text-muted)', padding: 60, textAlign: 'center' }}>Unlock your wallet to see rewards.</div>;
  }

  const hasSummary = summary.data && !summary.error;
  const hasRewards = rewards.data && !rewards.error && rewards.data.events?.length > 0;

  return (
    <div>
      {/* Summary */}
      <div style={{ background: 'var(--bg-surface)', padding: 24, borderBottom: '1px solid var(--border)', marginBottom: 16 }}>
        <h1 style={{ fontSize: 18, fontWeight: 600, marginBottom: 16 }}>💰 Rewards & Stakes</h1>

        <div style={{ display: 'flex', gap: 32, flexWrap: 'wrap', marginBottom: 12 }}>
          <div>
            <span className="label">Spendable Balance</span>
            <div><DranaAmount microdrana={balance} size={18} /></div>
          </div>
          <div>
            <span className="label">Locked in Post Stakes</span>
            <div><DranaAmount microdrana={postStakeBalance} size={18} /></div>
          </div>
          <div>
            <span className="label">Total Value</span>
            <div><DranaAmount microdrana={balance + postStakeBalance} size={18} /></div>
          </div>
        </div>

        {hasSummary && (
          <>
            <div style={{ borderTop: '1px solid var(--border)', paddingTop: 12, marginTop: 12, display: 'flex', gap: 32, flexWrap: 'wrap' }}>
              <div>
                <span className="label">Rewards (24h)</span>
                <div className="amber mono" style={{ fontSize: 16 }}>+{(summary.data!.last24h / 1_000_000).toFixed(2)}</div>
              </div>
              <div>
                <span className="label">Rewards (7d)</span>
                <div className="amber mono" style={{ fontSize: 16 }}>+{(summary.data!.last7d / 1_000_000).toFixed(2)}</div>
              </div>
              <div>
                <span className="label">Rewards (All Time)</span>
                <div className="amber mono" style={{ fontSize: 16 }}>+{(summary.data!.allTime / 1_000_000).toFixed(2)}</div>
              </div>
            </div>
          </>
        )}

        <p style={{ fontSize: 13, color: 'var(--text-muted)', marginTop: 12 }}>
          Staked across {stakes.data?.totalCount || 0} post{(stakes.data?.totalCount || 0) !== 1 ? 's' : ''}.
          Rewards are paid directly to your wallet when others stake on posts you're invested in.
        </p>
      </div>

      {/* Reward Feed */}
      {hasRewards && (
        <div style={{ marginBottom: 24 }}>
          <span className="label" style={{ display: 'block', marginBottom: 12 }}>RECENT REWARDS</span>
          {rewards.data!.events.map((ev, i) => (
            <div key={i} style={{ padding: '8px 0', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', fontSize: 13 }}>
              <div>
                <span className="amber mono">+{(ev.amount / 1_000_000).toFixed(2)}</span>
                <span style={{ color: 'var(--text-secondary)', marginLeft: 8 }}>
                  {ev.type === 'author' ? 'Author reward' : 'Staker reward'}
                </span>
              </div>
              <TimeAgo timestamp={ev.blockTime} />
            </div>
          ))}
        </div>
      )}

      {/* My Stakes */}
      <span className="label" style={{ display: 'block', marginBottom: 12 }}>
        MY STAKES ({stakes.data?.totalCount || 0} post{(stakes.data?.totalCount || 0) !== 1 ? 's' : ''} · <DranaAmount microdrana={stakes.data?.totalStaked || 0} size={12} />)
      </span>
      {(!stakes.data || stakes.data.stakes.length === 0) && (
        <p style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>
          No active stakes. Stake on posts to start earning rewards when others stake after you.
        </p>
      )}
      {stakes.data?.stakes.map(s => {
        const earned = rewardsPerPost.data?.[s.postId] ?? 0;
        return (
          <div key={s.postId} style={{ padding: '14px 16px', borderBottom: '1px solid var(--border)', background: 'var(--bg-surface)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', cursor: 'pointer' }}
            onClick={() => navigate(`post/${s.postId}`)}>
            <div>
              <span style={{ fontSize: 12 }}>📌 </span>
              <DranaAmount microdrana={s.amount} size={15} />
              {earned > 0 && (
                <span className="amber mono" style={{ fontSize: 12, marginLeft: 8 }}>
                  +{(earned / 1_000_000).toFixed(2)} earned
                </span>
              )}
            </div>
            <div style={{ textAlign: 'right' }}>
              <span className="mono" style={{ fontSize: 11, color: 'var(--text-muted)' }}>{s.postId.slice(0, 16)}...</span>
              <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>since block {s.height}</div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
