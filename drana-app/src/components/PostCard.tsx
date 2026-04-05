import type { RankedPost } from '../api/types';
import { useWallet } from '../wallet/WalletContext';
import { DranaAmount } from './DranaAmount';
import { TimeAgo } from './TimeAgo';
import { navigate } from '../router';

export function PostCard({ post, onBoost, onReply, onUnstake, isHighValue, userStake }: {
  post: RankedPost;
  onBoost: () => void;
  onReply: () => void;
  onUnstake?: () => void;
  isHighValue?: boolean;
  userStake?: number;
}) {
  const { resolveName } = useWallet();
  const resolvedName = resolveName(post.author);
  const authorDisplay = resolvedName || (post.author.length > 20
    ? post.author.slice(0, 10) + '...' + post.author.slice(-6)
    : post.author);

  const isWithdrawn = post.withdrawn;

  return (
    <div className="post-card" style={{
      borderLeft: isHighValue ? '3px solid var(--accent)' : 'none',
      background: isWithdrawn ? 'var(--bg-primary)' : isHighValue ? 'var(--accent-dim)' : 'var(--bg-surface)',
      opacity: isWithdrawn ? 0.5 : 1,
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 6 }}>
        <div>
          <span style={{ fontSize: 12, marginRight: 6 }}>📌</span>
          <DranaAmount microdrana={post.totalStaked} />
          <span style={{ fontSize: 12, color: 'var(--text-muted)', marginLeft: 6 }}>({post.stakerCount} staker{post.stakerCount !== 1 ? 's' : ''})</span>
        </div>
        <TimeAgo timestamp={post.createdAtTime} />
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
        <span style={{ fontSize: 14, fontWeight: 500, cursor: 'pointer' }}
          onClick={() => navigate(`profile/${post.author}`)}>{authorDisplay}</span>
        {post.channel && <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--channel)' }}>#{post.channel}</span>}
      </div>
      <p style={{ fontSize: 15, fontWeight: 500, marginBottom: 10, cursor: 'pointer', lineHeight: 1.4,
        overflow: 'hidden', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical' as any }}
        onClick={() => navigate(`post/${post.postId}`)}>
        {post.text}
      </p>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {post.replyCount > 0 && <span style={{ marginRight: 12 }}>💬 {post.replyCount}</span>}
          {userStake != null && userStake > 0 && (
            <span style={{ color: 'var(--success)' }}>Your stake: {(userStake / 1_000_000).toFixed(2)}</span>
          )}
        </div>
        {!isWithdrawn && (
          <div style={{ display: 'flex', gap: 8 }}>
            {userStake != null && userStake > 0 && onUnstake ? (
              <button className="action-btn" onClick={onUnstake} style={{ color: 'var(--error)' }}>Unstake</button>
            ) : (
              <button className="action-btn" onClick={onBoost}>Stake</button>
            )}
            <button className="action-btn" onClick={onReply}>Reply</button>
          </div>
        )}
        {isWithdrawn && <span style={{ fontSize: 12, color: 'var(--error)' }}>WITHDRAWN</span>}
      </div>

      <style>{`
        .post-card {
          padding: 16px 20px; border-bottom: 1px solid var(--border);
          transition: background var(--transition);
        }
        .post-card:hover { background: var(--bg-elevated) !important; }
        .action-btn {
          font-size: 13px; font-weight: 500; color: var(--accent);
          padding: 4px 12px; border: 1px solid transparent;
          transition: border-color var(--transition);
        }
        .action-btn:hover { border-color: var(--accent); }
      `}</style>
    </div>
  );
}
