import { useState, useEffect, useRef } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { getFeed, getChannels } from '../api/indexerApi';
import { getMyPostStakes } from '../api/nodeRpc';
import { useWallet } from '../wallet/WalletContext';
import { usePending } from '../wallet/PendingContext';
import { PostCard } from '../components/PostCard';
import { ChannelSearch } from '../components/ChannelSearch';
import { PendingPostCard } from '../components/PendingPostCard';
import { LoadingDots } from '../components/LoadingDots';
import { NewPostModal } from '../components/NewPostModal';
import { BoostModal } from '../components/BoostModal';
import { UnstakeModal } from '../components/UnstakeModal';
import { UnlockWalletModal, CreateWalletModal } from '../wallet/WalletModals';
import type { RankedPost } from '../api/types';

const PAGE_SIZE = 20;

export function Feed() {
  const wallet = useWallet();
  const { getPendingPosts } = usePending();
  const pendingPosts = getPendingPosts();
  const [strategy, setStrategy] = useState('trending');
  const [channel, setChannelState] = useState(() => {
    const match = window.location.hash.match(/[?&]channel=([^&]*)/);
    return match ? decodeURIComponent(match[1]) : '';
  });
  const setChannel = (ch: string) => {
    setChannelState(ch);
    window.history.replaceState(null, '', ch ? `#/?channel=${ch}` : '#/');
  };

  useEffect(() => {
    const handler = () => {
      const match = window.location.hash.match(/[?&]channel=([^&]*)/);
      setChannelState(match ? decodeURIComponent(match[1]) : '');
    };
    window.addEventListener('hashchange', handler);
    return () => window.removeEventListener('hashchange', handler);
  }, []);

  const [modal, setModal] = useState<'post' | 'reply' | 'boost' | 'unstake' | 'wallet-create' | 'wallet-unlock' | null>(null);
  const [selectedPost, setSelectedPost] = useState<RankedPost | null>(null);
  const [replyToId, setReplyToId] = useState<string | undefined>();
  const [unstakeAmount, setUnstakeAmount] = useState(0);

  const feed = useInfiniteQuery({
    queryKey: ['feed', strategy, channel],
    queryFn: ({ pageParam = 1 }) => getFeed({ strategy, channel, page: pageParam, pageSize: PAGE_SIZE }),
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.length * PAGE_SIZE;
      return loaded < lastPage.totalCount ? allPages.length + 1 : undefined;
    },
    initialPageParam: 1,
  });

  const channels = useQuery({ queryKey: ['channels'], queryFn: getChannels });

  // Fetch user's post stakes for showing Unstake buttons.
  const myStakes = useQuery({
    queryKey: ['my-post-stakes', wallet.address],
    queryFn: () => wallet.address ? getMyPostStakes(wallet.address) : Promise.resolve({ stakes: [], totalCount: 0, totalStaked: 0 }),
    enabled: !!wallet.address && wallet.isUnlocked,
  });
  const stakeMap = new Map<string, number>();
  myStakes.data?.stakes?.forEach(s => stakeMap.set(s.postId, s.amount));

  // Infinite scroll sentinel.
  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!sentinelRef.current || !feed.hasNextPage) return;
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting && !feed.isFetchingNextPage) {
        feed.fetchNextPage();
      }
    }, { rootMargin: '200px' });
    observer.observe(sentinelRef.current);
    return () => observer.disconnect();
  }, [feed.hasNextPage, feed.isFetchingNextPage, feed.fetchNextPage]);

  const requireWallet = (action: () => void) => {
    if (!wallet.hasWallet) { setModal('wallet-create'); return; }
    if (!wallet.isUnlocked) { setModal('wallet-unlock'); return; }
    action();
  };

  const allPosts = feed.data?.pages.flatMap(page => page.posts) ?? [];
  const topStaked = allPosts[0]?.totalStaked || 0;
  const highValueThreshold = topStaked * 0.5;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <button className="btn-primary" style={{ width: 'auto', padding: '8px 20px' }}
          onClick={() => requireWallet(() => setModal('post'))}>New Post</button>
        <select value={strategy} onChange={e => setStrategy(e.target.value)}
          style={{ width: 140, padding: '8px 12px', fontSize: 13 }}>
          <option value="trending">Trending</option>
          <option value="top">Top</option>
          <option value="new">New</option>
          <option value="controversial">Controversial</option>
        </select>
      </div>

      {channels.data && channels.data.length > 0 && (
        <ChannelSearch channels={channels.data} selected={channel} onSelect={setChannel} />
      )}

      {feed.isLoading && <div style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>Loading...</div>}
      {!feed.isLoading && allPosts.length === 0 && pendingPosts.length === 0 && (
        <div style={{ color: 'var(--text-muted)', padding: 60, textAlign: 'center' }}>
          <p style={{ marginBottom: 16 }}>No posts yet.</p>
          <button className="btn-primary" style={{ width: 'auto', padding: '8px 20px' }}
            onClick={() => requireWallet(() => setModal('post'))}>Create the first post</button>
        </div>
      )}

      {pendingPosts.map(tx => (
        <PendingPostCard key={tx.hash} tx={tx} />
      ))}
      {allPosts.map(post => (
        <PostCard key={post.postId} post={post}
          isHighValue={post.totalStaked > highValueThreshold && post.totalStaked > 0}
          userStake={stakeMap.get(post.postId) || 0}
          onBoost={() => requireWallet(() => { setSelectedPost(post); setModal('boost'); })}
          onReply={() => requireWallet(() => { setReplyToId(post.postId); setModal('reply'); })}
          onUnstake={() => requireWallet(() => { setSelectedPost(post); setUnstakeAmount(stakeMap.get(post.postId) || 0); setModal('unstake'); })} />
      ))}

      {feed.hasNextPage && <div ref={sentinelRef} style={{ height: 1 }} />}
      {feed.isFetchingNextPage && <LoadingDots />}

      {modal === 'post' && <NewPostModal onClose={() => setModal(null)} />}
      {modal === 'reply' && <NewPostModal onClose={() => { setModal(null); setReplyToId(undefined); }} parentPostId={replyToId} />}
      {modal === 'boost' && selectedPost && <BoostModal post={selectedPost} onClose={() => { setModal(null); setSelectedPost(null); }} />}
      {modal === 'unstake' && selectedPost && <UnstakeModal post={selectedPost} stakeAmount={unstakeAmount} onClose={() => { setModal(null); setSelectedPost(null); }} />}
      {modal === 'wallet-create' && <CreateWalletModal onClose={() => setModal(null)} />}
      {modal === 'wallet-unlock' && <UnlockWalletModal onClose={() => setModal(null)} />}
    </div>
  );
}
