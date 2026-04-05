package indexer

// IndexedPost is a post with derived fields computed by the indexer.
type IndexedPost struct {
	PostID             string `json:"postId"`
	Author             string `json:"author"`
	Text               string `json:"text"`
	Channel            string `json:"channel,omitempty"`
	ParentPostID       string `json:"parentPostId,omitempty"`
	CreatedAtHeight    uint64 `json:"createdAtHeight,string"`
	CreatedAtTime      int64  `json:"createdAtTime"`
	TotalStaked        uint64 `json:"totalStaked,string"`
	AuthorStaked       uint64 `json:"authorStaked,string"`
	ThirdPartyStaked   uint64 `json:"thirdPartyStaked,string"`
	StakerCount        uint64 `json:"stakerCount,string"`
	UniqueBoosterCount int    `json:"uniqueBoosterCount"`
	LastBoostAtHeight  uint64 `json:"lastBoostAtHeight,string"`
	ReplyCount         int    `json:"replyCount"`
	Withdrawn          bool   `json:"withdrawn"`
	TotalBurned        uint64 `json:"totalBurned,string"`
}

// ChannelInfo holds stats for a channel.
type ChannelInfo struct {
	Channel   string `json:"channel"`
	PostCount int    `json:"postCount"`
}

// RankedPost extends IndexedPost with a computed score.
type RankedPost struct {
	IndexedPost
	Score float64 `json:"score"`
}

// IndexedBoost records a single boost event.
type IndexedBoost struct {
	PostID       string `json:"postId"`
	Booster      string `json:"booster"`
	Amount       uint64 `json:"amount,string"`
	AuthorReward uint64 `json:"authorReward,string"`
	StakerReward uint64 `json:"stakerReward,string"`
	BurnAmount   uint64 `json:"burnAmount,string"`
	StakedAmount uint64 `json:"stakedAmount,string"`
	BlockHeight  uint64 `json:"blockHeight,string"`
	BlockTime    int64  `json:"blockTime"`
	TxHash       string `json:"txHash"`
}

// RewardEvent records a single reward payment to a recipient.
type RewardEvent struct {
	PostID         string `json:"postId"`
	Recipient      string `json:"recipient"`
	Amount         uint64 `json:"amount,string"`
	BlockHeight    uint64 `json:"blockHeight,string"`
	BlockTime      int64  `json:"blockTime"`
	TriggerTx      string `json:"triggerTx"`
	TriggerAddress string `json:"triggerAddress"`
	Type           string `json:"type"`
}

// PostStakeRecord tracks a staker's position on a post.
type PostStakeRecord struct {
	PostID string `json:"postId"`
	Staker string `json:"staker"`
	Amount uint64 `json:"amount,string"`
	Height uint64 `json:"height,string"`
}

// IndexedTransfer records a transfer event.
type IndexedTransfer struct {
	TxHash      string `json:"txHash"`
	Sender      string `json:"sender"`
	Recipient   string `json:"recipient"`
	Amount      uint64 `json:"amount,string"`
	BlockHeight uint64 `json:"blockHeight,string"`
	BlockTime   int64  `json:"blockTime"`
}

// ChainStats holds global indexer statistics.
type ChainStats struct {
	LatestHeight      uint64 `json:"latestHeight,string"`
	TotalPosts        int    `json:"totalPosts"`
	TotalBoosts       int    `json:"totalBoosts"`
	TotalTransfers    int    `json:"totalTransfers"`
	TotalBurned       uint64 `json:"totalBurned,string"`
	TotalIssued       uint64 `json:"totalIssued,string"`
	CirculatingSupply uint64 `json:"circulatingSupply,string"`
}

// AuthorProfile holds aggregate stats for an author.
type AuthorProfile struct {
	Address            string `json:"address"`
	PostCount          int    `json:"postCount"`
	TotalStaked        uint64 `json:"totalStaked,string"`
	TotalReceived      uint64 `json:"totalReceived,string"`
	UniqueBoosterCount int    `json:"uniqueBoosterCount"`
}

// LeaderboardEntry is a row in the author leaderboard.
type LeaderboardEntry struct {
	Address       string `json:"address"`
	TotalReceived uint64 `json:"totalReceived,string"`
	PostCount     int    `json:"postCount"`
	StakerCount   int    `json:"stakerCount"`
}
