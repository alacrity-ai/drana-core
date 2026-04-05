package rpc

// --- Chain / Node ---

type NodeInfoResponse struct {
	ChainID              string `json:"chainId"`
	LatestHeight         uint64 `json:"latestHeight,string"`
	LatestHash           string `json:"latestHash"`
	GenesisTime          int64  `json:"genesisTime"`
	BlockInterval        int    `json:"blockIntervalSec"`
	BlockReward          uint64 `json:"blockReward,string"`
	BurnedSupply         uint64 `json:"burnedSupply,string"`
	IssuedSupply         uint64 `json:"issuedSupply,string"`
	ValidatorCount       int    `json:"validatorCount"`
	CurrentEpoch         uint64 `json:"currentEpoch,string"`
	EpochLength          uint64 `json:"epochLength,string"`
	BlocksUntilNextEpoch uint64 `json:"blocksUntilNextEpoch,string"`
}

type BlockResponse struct {
	Height       uint64                `json:"height,string"`
	Hash         string                `json:"hash"`
	PrevHash     string                `json:"prevHash"`
	ProposerAddr string                `json:"proposerAddr"`
	Timestamp    int64                 `json:"timestamp"`
	StateRoot    string                `json:"stateRoot"`
	TxRoot       string                `json:"txRoot"`
	TxCount      int                   `json:"txCount"`
	Transactions []TransactionResponse `json:"transactions,omitempty"`
}

// --- Accounts ---

type AccountResponse struct {
	Address          string `json:"address"`
	Balance          uint64 `json:"balance,string"`
	Nonce            uint64 `json:"nonce,string"`
	Name             string `json:"name,omitempty"`
	StakedBalance    uint64 `json:"stakedBalance,string"`
	PostStakeBalance uint64 `json:"postStakeBalance,string"`
}

// --- Posts ---

type PostResponse struct {
	PostID          string `json:"postId"`
	Author          string `json:"author"`
	Text            string `json:"text"`
	Channel         string `json:"channel,omitempty"`
	ParentPostID    string `json:"parentPostId,omitempty"`
	CreatedAtHeight uint64 `json:"createdAtHeight,string"`
	CreatedAtTime   int64  `json:"createdAtTime"`
	TotalStaked     uint64 `json:"totalStaked,string"`
	TotalBurned     uint64 `json:"totalBurned,string"`
	StakerCount     uint64 `json:"stakerCount,string"`
	Withdrawn       bool   `json:"withdrawn,omitempty"`
}

type PostListResponse struct {
	Posts      []PostResponse `json:"posts"`
	TotalCount int            `json:"totalCount"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
}

// --- Transactions ---

type TransactionResponse struct {
	Hash         string `json:"hash"`
	Type         string `json:"type"`
	Sender       string `json:"sender"`
	Recipient    string `json:"recipient,omitempty"`
	PostID       string `json:"postId,omitempty"`
	ParentPostID string `json:"parentPostId,omitempty"`
	Text         string `json:"text,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Amount       uint64 `json:"amount,string"`
	Nonce        uint64 `json:"nonce,string"`
	BlockHeight  uint64 `json:"blockHeight,string,omitempty"`
}

type SubmitTxRequest struct {
	Type         string `json:"type"`
	Sender       string `json:"sender"`
	Recipient    string `json:"recipient,omitempty"`
	PostID       string `json:"postId,omitempty"`
	Text         string `json:"text,omitempty"`
	Channel      string `json:"channel,omitempty"`
	ParentPostID string `json:"parentPostId,omitempty"`
	Amount       uint64 `json:"amount,string"`
	Nonce        uint64 `json:"nonce,string"`
	Signature    string `json:"signature"`
	PubKey       string `json:"pubKey"`
}

type SubmitTxResponse struct {
	Accepted bool   `json:"accepted"`
	TxHash   string `json:"txHash,omitempty"`
	Error    string `json:"error,omitempty"`
}

type TxStatusResponse struct {
	Hash        string `json:"hash"`
	Status      string `json:"status"`
	BlockHeight uint64 `json:"blockHeight,string,omitempty"`
}

// --- Mempool ---

type MempoolResponse struct {
	Transactions []TransactionResponse `json:"transactions"`
	Count        int                   `json:"count"`
}

// --- Network ---

type ValidatorResponse struct {
	Address       string `json:"address"`
	Name          string `json:"name,omitempty"`
	PubKey        string `json:"pubKey"`
	StakedBalance uint64 `json:"stakedBalance,string"`
}

type PeerResponse struct {
	Endpoint string `json:"endpoint"`
}

type UnbondingEntryResponse struct {
	Amount        uint64 `json:"amount,string"`
	ReleaseHeight uint64 `json:"releaseHeight,string"`
}

type UnbondingResponse struct {
	Address string                   `json:"address"`
	Entries []UnbondingEntryResponse `json:"entries"`
	Total   uint64                   `json:"total,string"`
}

// --- Common ---

type ErrorResponse struct {
	Error string `json:"error"`
}
