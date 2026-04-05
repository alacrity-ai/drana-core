import { INDEXER_API } from './config';
import { numericReviver } from './numericReviver';
import type { FeedResponse, ChannelInfo, RankedPost, BoostHistoryResponse, AuthorProfile, StatsResponse, LeaderboardEntry, RewardEvent, RewardSummary } from './types';

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${INDEXER_API}${path}`);
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    let msg = `HTTP ${res.status}`;
    try { msg = JSON.parse(text).error || msg; } catch {}
    throw new Error(msg);
  }
  const text = await res.text();
  return JSON.parse(text, numericReviver);
}

export function getFeed(params: { strategy?: string; channel?: string; page?: number; pageSize?: number } = {}) {
  const q = new URLSearchParams();
  if (params.strategy) q.set('strategy', params.strategy);
  if (params.channel) q.set('channel', params.channel);
  q.set('page', String(params.page || 1));
  q.set('pageSize', String(params.pageSize || 20));
  return get<FeedResponse>(`/v1/feed?${q}`);
}

export function getFeedByAuthor(address: string, params: { strategy?: string; page?: number } = {}) {
  const q = new URLSearchParams();
  if (params.strategy) q.set('strategy', params.strategy);
  if (params.page) q.set('page', String(params.page));
  return get<FeedResponse>(`/v1/feed/author/${address}?${q}`);
}

export const getChannels = () => get<ChannelInfo[]>('/v1/channels');
export const getPost = (id: string) => get<RankedPost>(`/v1/posts/${id}`);

export function getPostBoosts(id: string, page = 1) {
  return get<BoostHistoryResponse>(`/v1/posts/${id}/boosts?page=${page}`);
}

export function getPostReplies(id: string, page = 1) {
  return get<{ replies: RankedPost[]; totalCount: number; page: number; pageSize: number }>(`/v1/posts/${id}/replies?page=${page}`);
}

export const getAuthorProfile = (addr: string) => get<AuthorProfile>(`/v1/authors/${addr}`);
export const getStats = () => get<StatsResponse>('/v1/stats');

export function getRewards(address: string, sinceHeight = 0, page = 1) {
  return get<{ events: RewardEvent[]; totalCount: number; totalAmount: number }>(
    `/v1/rewards/${address}?since=${sinceHeight}&page=${page}`);
}
export const getRewardSummary = (address: string) =>
  get<RewardSummary>(`/v1/rewards/${address}/summary`);

export function getLeaderboard(page = 1) {
  return get<{ authors: LeaderboardEntry[]; totalCount: number }>(`/v1/leaderboard?page=${page}`);
}

export const getRewardsForPost = (address: string, postId: string) =>
  get<{ totalReward: number }>(`/v1/rewards/${address}/post/${postId}`);
