package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/drana-chain/drana/internal/rpc"
	"github.com/drana-chain/drana/internal/types"
)

// Follower polls the node RPC for new blocks and indexes their contents.
type Follower struct {
	nodeRPC        string
	db             *DB
	pollInterval   time.Duration
	postFeePercent uint64 // total fee on create_post (e.g. 6)
	boostBurnPct   uint64 // burn % of boost amount (e.g. 3)
	boostAuthorPct uint64 // author reward % of boost amount (e.g. 2)
	boostStakerPct uint64 // staker reward % of boost amount (e.g. 1)
}

// NewFollower creates a chain follower.
func NewFollower(nodeRPC string, db *DB, pollInterval time.Duration, postFeePct, boostBurnPct, boostAuthorPct, boostStakerPct uint64) *Follower {
	return &Follower{
		nodeRPC:        nodeRPC,
		db:             db,
		pollInterval:   pollInterval,
		postFeePercent: postFeePct,
		boostBurnPct:   boostBurnPct,
		boostAuthorPct: boostAuthorPct,
		boostStakerPct: boostStakerPct,
	}
}

// Run starts the follower loop. Blocks until ctx is cancelled.
func (f *Follower) Run(ctx context.Context) error {
	log.Printf("indexer: starting follower against %s", f.nodeRPC)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := f.poll(ctx); err != nil {
			log.Printf("indexer: poll error: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.pollInterval):
		}
	}
}

func (f *Follower) poll(ctx context.Context) error {
	lastH, err := f.db.GetLastIndexedHeight()
	if err != nil {
		return fmt.Errorf("get last height: %w", err)
	}

	// Get current chain height.
	var nodeInfo rpc.NodeInfoResponse
	if err := httpGetJSON(f.nodeRPC+"/v1/node/info", &nodeInfo); err != nil {
		return fmt.Errorf("get node info: %w", err)
	}

	if nodeInfo.LatestHeight <= lastH {
		return nil // up to date
	}

	for h := lastH + 1; h <= nodeInfo.LatestHeight; h++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := f.IndexBlock(h); err != nil {
			return fmt.Errorf("index block %d: %w", h, err)
		}
	}
	return nil
}

// IndexBlock fetches and indexes a single block.
func (f *Follower) IndexBlock(height uint64) error {
	url := fmt.Sprintf("%s/v1/blocks/%d?full=true", f.nodeRPC, height)
	var block rpc.BlockResponse
	if err := httpGetJSON(url, &block); err != nil {
		return fmt.Errorf("fetch block %d: %w", height, err)
	}

	for _, tx := range block.Transactions {
		switch tx.Type {
		case "transfer":
			f.db.InsertTransfer(&IndexedTransfer{
				TxHash:      tx.Hash,
				Sender:      tx.Sender,
				Recipient:   tx.Recipient,
				Amount:      tx.Amount,
				BlockHeight: height,
				BlockTime:   block.Timestamp,
			})

		case "create_post":
			postID := f.findPostID(tx.Sender, tx.Nonce)
			if postID == "" {
				log.Printf("indexer: could not find post ID for tx %s", tx.Hash)
				continue
			}

			// Compute staked amount (total minus fee).
			fee := types.MulDiv(tx.Amount, f.postFeePercent, 100)
			stakedAmount := tx.Amount - fee

			f.db.InsertPost(&IndexedPost{
				PostID:          postID,
				Author:          tx.Sender,
				Text:            tx.Text,
				Channel:         tx.Channel,
				ParentPostID:    tx.ParentPostID,
				CreatedAtHeight: height,
				CreatedAtTime:   block.Timestamp,
				TotalStaked:     stakedAmount,
				AuthorStaked:    stakedAmount,
				StakerCount:     1,
				TotalBurned:     fee,
			})

			// Track author's stake position.
			f.db.UpsertPostStake(postID, tx.Sender, stakedAmount, height)

		case "boost_post":
			if tx.PostID == "" {
				continue
			}

			post, _ := f.db.GetPost(tx.PostID)
			author := ""
			if post != nil {
				author = post.Author
			}

			// Compute fee breakdown.
			burnAmount := types.MulDiv(tx.Amount, f.boostBurnPct, 100)
			authorReward := types.MulDiv(tx.Amount, f.boostAuthorPct, 100)
			stakerReward := types.MulDiv(tx.Amount, f.boostStakerPct, 100)
			stakedAmount := tx.Amount - burnAmount - authorReward - stakerReward

			// Insert boost with breakdown.
			if err := f.db.InsertBoost(&IndexedBoost{
				PostID:       tx.PostID,
				Booster:      tx.Sender,
				Amount:       tx.Amount,
				AuthorReward: authorReward,
				StakerReward: stakerReward,
				BurnAmount:   burnAmount,
				StakedAmount: stakedAmount,
				BlockHeight:  height,
				BlockTime:    block.Timestamp,
				TxHash:       tx.Hash,
			}, author); err != nil {
				log.Printf("indexer: InsertBoost failed for tx %s: %v", tx.Hash, err)
			}

			// Record author reward event.
			if authorReward > 0 && author != "" {
				f.db.InsertRewardEvent(tx.PostID, author, tx.Hash, tx.Sender, "author",
					authorReward, height, block.Timestamp)
			}

			// Record staker reward events (pro-rata split).
			if stakerReward > 0 && post != nil {
				stakers := f.db.GetPostStakePositions(tx.PostID)
				var totalStaked uint64
				for _, s := range stakers {
					totalStaked += s.Amount
				}
				if totalStaked > 0 {
					for _, s := range stakers {
						share := types.MulDiv(stakerReward, s.Amount, totalStaked)
						if share > 0 {
							f.db.InsertRewardEvent(tx.PostID, s.Staker, tx.Hash, tx.Sender, "staker",
								share, height, block.Timestamp)
						}
					}
				}
			}

			// Track booster's stake position.
			f.db.UpsertPostStake(tx.PostID, tx.Sender, stakedAmount, height)

		case "unstake_post":
			if tx.PostID == "" {
				continue
			}
			post, _ := f.db.GetPost(tx.PostID)
			if post != nil && tx.Sender == post.Author {
				// Author withdrawal — remove all stakes, mark withdrawn.
				f.db.RemoveAllPostStakes(tx.PostID)
				f.db.exec("UPDATE posts SET withdrawn = 1, total_staked = 0, author_staked = 0, third_party_staked = 0, staker_count = 0 WHERE post_id = ?", tx.PostID)
			} else {
				// Regular unstake — remove this staker's position.
				f.db.RemovePostStake(tx.PostID, tx.Sender)
			}

		case "register_name":
			// Stored in consensus state, queryable via node RPC.

		case "stake", "unstake":
			// Validator staking events are reflected in account state.
		}
	}

	return f.db.SetLastIndexedHeight(height)
}

// findPostID queries the node to find the post created by a CreatePost tx.
// It looks up the post by computing the PostID from sender address + nonce.
func (f *Follower) findPostID(senderDisplay string, nonce uint64) string {
	// We need to compute PostID = SHA-256(address_bytes || nonce_bytes).
	// But we only have the display address. Let's parse it and compute.
	addr, err := parseAddressForIndexer(senderDisplay)
	if err != nil {
		return ""
	}
	return computePostIDHex(addr, nonce)
}

func httpGetJSON(url string, out interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
