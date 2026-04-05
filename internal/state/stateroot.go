package state

import (
	"sort"

	"github.com/drana-chain/drana/internal/types"
)

// ComputeStateRoot produces a deterministic hash of the entire world state.
// Accounts and posts are sorted by their keys to ensure order-independence.
func ComputeStateRoot(ws *WorldState) [32]byte {
	hw := types.NewHashWriter()

	// Sort accounts by address bytes.
	accounts := ws.AllAccounts()
	sort.Slice(accounts, func(i, j int) bool {
		return compareBytes(accounts[i].Address[:], accounts[j].Address[:]) < 0
	})
	hw.WriteUint64(uint64(len(accounts)))
	for _, a := range accounts {
		hw.WriteBytes(a.Address[:])
		hw.WriteUint64(a.Balance)
		hw.WriteUint64(a.Nonce)
		hw.WriteString(a.Name)
		hw.WriteUint64(a.StakedBalance)
	}

	// Sort posts by PostID bytes.
	posts := ws.AllPosts()
	sort.Slice(posts, func(i, j int) bool {
		return compareBytes(posts[i].PostID[:], posts[j].PostID[:]) < 0
	})
	hw.WriteUint64(uint64(len(posts)))
	for _, p := range posts {
		hw.WriteBytes(p.PostID[:])
		hw.WriteBytes(p.Author[:])
		hw.WriteString(p.Text)
		hw.WriteString(p.Channel)
		hw.WriteBytes(p.ParentPostID[:])
		hw.WriteUint64(p.CreatedAtHeight)
		hw.WriteInt64(p.CreatedAtTime)
		hw.WriteUint64(p.TotalStaked)
		hw.WriteUint64(p.TotalBurned)
		hw.WriteUint64(p.StakerCount)
		if p.Withdrawn {
			hw.WriteUint64(1)
		} else {
			hw.WriteUint64(0)
		}
	}

	// Post stakes (sorted by postID + staker).
	allStakes := ws.AllPostStakes()
	sort.Slice(allStakes, func(i, j int) bool {
		c := compareBytes(allStakes[i].PostID[:], allStakes[j].PostID[:])
		if c != 0 {
			return c < 0
		}
		return compareBytes(allStakes[i].Staker[:], allStakes[j].Staker[:]) < 0
	})
	hw.WriteUint64(uint64(len(allStakes)))
	for _, ps := range allStakes {
		hw.WriteBytes(ps.PostID[:])
		hw.WriteBytes(ps.Staker[:])
		hw.WriteUint64(ps.Amount)
		hw.WriteUint64(ps.Height)
	}

	// Active validators (already sorted).
	activeVals := ws.GetActiveValidators()
	hw.WriteUint64(uint64(len(activeVals)))
	for _, v := range activeVals {
		hw.WriteBytes(v.Address[:])
		hw.WriteBytes(v.PubKey[:])
		hw.WriteUint64(v.StakedBalance)
	}

	// Unbonding queue.
	unbonding := ws.GetUnbondingQueue()
	hw.WriteUint64(uint64(len(unbonding)))
	for _, u := range unbonding {
		hw.WriteBytes(u.Address[:])
		hw.WriteUint64(u.Amount)
		hw.WriteUint64(u.ReleaseHeight)
	}

	hw.WriteUint64(ws.GetCurrentEpoch())
	hw.WriteUint64(ws.GetBurnedSupply())
	hw.WriteUint64(ws.GetIssuedSupply())
	hw.WriteUint64(ws.GetChainHeight())

	return hw.Sum256()
}

func compareBytes(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return len(a) - len(b)
}
