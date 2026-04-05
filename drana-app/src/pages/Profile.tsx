import { useQuery } from '@tanstack/react-query';
import { getAccount } from '../api/nodeRpc';
import { getFeedByAuthor, getAuthorProfile } from '../api/indexerApi';
import { DranaAmount } from '../components/DranaAmount';
import { PostCard } from '../components/PostCard';

export function Profile({ address }: { address: string }) {
  const account = useQuery({ queryKey: ['account', address], queryFn: () => getAccount(address) });
  const profile = useQuery({ queryKey: ['author-profile', address], queryFn: () => getAuthorProfile(address) });
  const posts = useQuery({ queryKey: ['author-posts', address], queryFn: () => getFeedByAuthor(address, { strategy: 'new' }) });

  const truncated = address.length > 20 ? address.slice(0, 14) + '...' + address.slice(-8) : address;

  return (
    <div>
      <div style={{ background: 'var(--bg-surface)', padding: 24, borderBottom: '1px solid var(--border)', marginBottom: 16 }}>
        <h1 style={{ fontSize: 20, fontWeight: 600, marginBottom: 4 }}>
          {account.data?.name || truncated}
        </h1>
        {account.data?.name && (
          <p className="mono" style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 12, wordBreak: 'break-all' }}>{address}</p>
        )}
        <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
          {account.data && (
            <>
              <div>
                <span className="label">Balance</span>
                <div><DranaAmount microdrana={account.data.balance} size={16} /></div>
              </div>
              {account.data.stakedBalance > 0 && (
                <div>
                  <span className="label">Validator Staked</span>
                  <div><DranaAmount microdrana={account.data.stakedBalance} size={16} /></div>
                </div>
              )}
              {account.data.postStakeBalance > 0 && (
                <div>
                  <span className="label">Post Stakes</span>
                  <div><DranaAmount microdrana={account.data.postStakeBalance} size={16} /></div>
                </div>
              )}
            </>
          )}
          {profile.data && (
            <>
              <div>
                <span className="label">Posts</span>
                <div style={{ fontSize: 16, fontWeight: 500 }}>{profile.data.postCount}</div>
              </div>
              <div>
                <span className="label">Rewards Earned</span>
                <div><DranaAmount microdrana={profile.data.totalReceived} size={16} /></div>
              </div>
            </>
          )}
        </div>
      </div>

      <span className="label" style={{ display: 'block', marginBottom: 12 }}>POSTS</span>
      {posts.data?.posts?.length === 0 && (
        <p style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>No posts yet.</p>
      )}
      {posts.data?.posts?.map(post => (
        <PostCard key={post.postId} post={post} onBoost={() => {}} onReply={() => {}} />
      ))}
    </div>
  );
}
