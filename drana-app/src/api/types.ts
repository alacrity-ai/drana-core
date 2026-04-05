export interface NodeInfo {
  chainId: string;
  latestHeight: number;
  latestHash: string;
  genesisTime: number;
  blockIntervalSec: number;
  blockReward: number;
  burnedSupply: number;
  issuedSupply: number;
  validatorCount: number;
  currentEpoch: number;
  epochLength: number;
  blocksUntilNextEpoch: number;
}

export interface Account {
  address: string;
  balance: number;
  nonce: number;
  name?: string;
  stakedBalance: number;
  postStakeBalance: number;
}

export interface UnbondingEntry { amount: number; releaseHeight: number; }
export interface UnbondingResponse { address: string; entries: UnbondingEntry[]; total: number; }

export interface Post {
  postId: string;
  author: string;
  text: string;
  channel?: string;
  parentPostId?: string;
  createdAtHeight: number;
  createdAtTime: number;
  totalStaked: number;
  totalBurned: number;
  stakerCount: number;
  withdrawn?: boolean;
}

export interface RankedPost extends Post {
  authorStaked: number;
  thirdPartyStaked: number;
  uniqueBoosterCount: number;
  lastBoostAtHeight: number;
  replyCount: number;
  score: number;
}

export interface PostList { posts: Post[]; totalCount: number; page: number; pageSize: number; }
export interface FeedResponse { posts: RankedPost[]; totalCount: number; page: number; pageSize: number; strategy: string; }

export interface IndexedBoost {
  postId: string;
  booster: string;
  amount: number;
  blockHeight: number;
  blockTime: number;
  txHash: string;
}
export interface BoostHistoryResponse { boosts: IndexedBoost[]; totalCount: number; page: number; pageSize: number; }

export interface ChannelInfo { channel: string; postCount: number; }

export interface AuthorProfile {
  address: string;
  postCount: number;
  totalStaked: number;
  totalReceived: number;
  uniqueBoosterCount: number;
}

export interface StatsResponse {
  latestHeight: number;
  totalPosts: number;
  totalBoosts: number;
  totalTransfers: number;
  totalBurned: number;
  totalIssued: number;
  circulatingSupply: number;
}

export interface LeaderboardEntry { address: string; totalReceived: number; postCount: number; stakerCount: number; }
export interface Validator { address: string; name?: string; pubKey: string; stakedBalance: number; }

export interface PostStakePosition { postId: string; amount: number; height: number; }
export interface MyStakesResponse { stakes: PostStakePosition[]; totalCount: number; totalStaked: number; }
export interface StakerInfo { address: string; amount: number; height: number; }

export interface RewardEvent {
  postId: string; recipient: string; amount: number;
  blockHeight: number; blockTime: number; triggerTx: string; triggerAddress: string; type: 'author' | 'staker';
}
export interface RewardSummary { last24h: number; last7d: number; allTime: number; postCount: number; totalStaked: number; }

export interface TransactionResponse {
  hash: string; type: string; sender: string; recipient?: string;
  postId?: string; parentPostId?: string; text?: string; channel?: string;
  amount: number; nonce: number; blockHeight?: number;
}

export interface SubmitTxRequest {
  type: string; sender: string; recipient?: string; postId?: string;
  parentPostId?: string; text?: string; channel?: string;
  amount: string; nonce: string; signature: string; pubKey: string;
}

export interface SubmitTxResponse { accepted: boolean; txHash?: string; error?: string; }
export interface TxStatus { hash: string; status: 'confirmed' | 'pending' | 'unknown'; blockHeight?: number; }
