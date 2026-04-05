package consensus

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/mempool"
	"github.com/drana-chain/drana/internal/p2p"
	pb "github.com/drana-chain/drana/internal/proto/pb"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/store"
	"github.com/drana-chain/drana/internal/types"
)

// Engine orchestrates the consensus lifecycle for a single validator node.
type Engine struct {
	mu sync.RWMutex

	Params         *types.GenesisConfig
	GenesisValSet  []types.GenesisValidator // kept for genesis init only
	PrivKey        crypto.PrivateKey
	Address        crypto.Address

	State      *state.WorldState
	Executor   *state.Executor
	Mempool    *mempool.Mempool
	BlockStore *store.BlockStore
	KVStore    *store.KVStore
	Peers      *p2p.PeerManager

	currentHeight uint64
	lastBlock     *types.Block

	// BlockInterval can be overridden for testing.
	BlockInterval time.Duration

	// blockCommitted is signaled whenever a block is committed (by any path).
	blockCommitted chan struct{}
}

// NewEngine creates a consensus engine.
func NewEngine(
	params *types.GenesisConfig,
	validators []types.GenesisValidator,
	privKey crypto.PrivateKey,
	address crypto.Address,
	ws *state.WorldState,
	mp *mempool.Mempool,
	bs *store.BlockStore,
	kv *store.KVStore,
	peers *p2p.PeerManager,
) *Engine {
	interval := time.Duration(params.BlockIntervalSec) * time.Second
	if interval == 0 {
		interval = 120 * time.Second
	}
	eng := &Engine{
		Params:         params,
		GenesisValSet:  validators,
		PrivKey:        privKey,
		Address:        address,
		State:         ws,
		Executor:      &state.Executor{Params: params},
		Mempool:       mp,
		BlockStore:    bs,
		KVStore:       kv,
		Peers:         peers,
		currentHeight:  ws.GetChainHeight(),
		BlockInterval:  interval,
		blockCommitted: make(chan struct{}, 1),
	}
	// Give mempool access to current state for balance/nonce checks.
	mp.SetStateChecker(eng)
	return eng
}

// GetAccount implements mempool.AccountChecker using the engine's current state.
func (e *Engine) GetAccount(addr crypto.Address) (*types.Account, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.State.GetAccount(addr)
}

// activeValidators returns the current active validator set from state.
// Falls back to genesis validators if no PoS set is established.
func (e *Engine) activeValidators() []types.ValidatorStake {
	vs := e.State.GetActiveValidators()
	if len(vs) > 0 {
		return vs
	}
	return ValidatorsFromGenesis(e.GenesisValSet)
}

// SetLastBlock sets the last finalized block (loaded from storage on startup).
func (e *Engine) SetLastBlock(b *types.Block) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastBlock = b
	if b != nil {
		e.currentHeight = b.Header.Height
	}
}

// CurrentHeight returns the current chain height.
func (e *Engine) CurrentHeight() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentHeight
}

// CurrentState returns the current world state.
func (e *Engine) CurrentState() *state.WorldState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.State
}

// Run starts the consensus main loop. Blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	log.Printf("consensus: starting at height %d", e.currentHeight)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		nextHeight := e.currentHeight + 1
		if IsProposer(e.activeValidators(), nextHeight, e.Address) {
			e.waitForBlockInterval(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := e.proposeBlock(ctx, nextHeight); err != nil {
				log.Printf("consensus: propose failed at height %d: %v", nextHeight, err)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
		} else {
			// Wait for block to be committed via OnFinalizedBlock handler, or timeout.
			timeout := e.BlockInterval + 10*time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-e.blockCommitted:
				// Block was committed by handler — loop back to check next height.
			case <-time.After(timeout):
				log.Printf("consensus: timeout waiting for block %d", nextHeight)
			}
		}
	}
}

func (e *Engine) waitForBlockInterval(ctx context.Context) {
	e.mu.RLock()
	last := e.lastBlock
	e.mu.RUnlock()

	if last == nil {
		return // genesis — propose immediately
	}

	target := time.Unix(last.Header.Timestamp, 0).Add(e.BlockInterval)
	wait := time.Until(target)
	if wait <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

func (e *Engine) proposeBlock(ctx context.Context, height uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	log.Printf("consensus: proposing block %d", height)

	// Reap transactions from mempool.
	txs := e.Mempool.ReapForBlock(e.State, e.Params, e.Params.MaxTxPerBlock)
	txRoot := types.ComputeTxRoot(txs)

	var prevHash [32]byte
	if e.lastBlock != nil {
		prevHash = e.lastBlock.Header.Hash()
	}

	block := &types.Block{
		Header: types.BlockHeader{
			Height:       height,
			PrevHash:     prevHash,
			ProposerAddr: e.Address,
			Timestamp:    time.Now().Unix(),
			TxRoot:       txRoot,
		},
		Transactions: txs,
	}

	// Trial execute to get state root.
	newState, err := e.Executor.ApplyBlock(e.State, block)
	if err != nil {
		return fmt.Errorf("trial execute: %w", err)
	}
	block.Header.StateRoot = state.ComputeStateRoot(newState)

	// Self-vote.
	blockHash := block.Header.Hash()
	selfVote := types.BlockVote{
		Height:    height,
		BlockHash: blockHash,
		VoterAddr: e.Address,
	}
	types.SignBlockVote(&selfVote, e.PrivKey)

	votes := []types.BlockVote{selfVote}

	// Send proposal to peers and collect votes.
	proposal := &pb.BlockProposal{Block: p2p.BlockToProto(block)}
	for _, peer := range e.Peers.Peers() {
		peerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		pbVote, err := peer.ProposeBlock(peerCtx, proposal)
		cancel()
		if err != nil {
			log.Printf("consensus: peer %s vote failed: %v", peer.Addr, err)
			continue
		}
		votes = append(votes, p2p.VoteFromProto(pbVote))
	}

	// Check stake-weighted quorum.
	activeSet := e.activeValidators()
	stakeMap := make(map[crypto.Address]uint64)
	for _, v := range activeSet {
		stakeMap[v.Address] = v.StakedBalance
	}
	var voteStake uint64
	for _, v := range votes {
		if types.VerifyBlockVote(&v) && v.Height == height && v.BlockHash == blockHash {
			voteStake += stakeMap[v.VoterAddr]
		}
	}
	required := QuorumThreshold(activeSet)
	if voteStake < required {
		return fmt.Errorf("insufficient vote stake: %d/%d", voteStake, required)
	}

	// Assemble QC.
	qc := &types.QuorumCertificate{
		Height:    height,
		BlockHash: blockHash,
		Votes:     votes,
	}
	block.QC = qc

	// Commit locally.
	if err := e.commitBlock(block, newState); err != nil {
		return err
	}

	// Notify peers of finalized block.
	fb := &pb.FinalizedBlock{
		Block: p2p.BlockToProto(block),
		Qc:    p2p.QCToProto(qc),
	}
	for _, peer := range e.Peers.Peers() {
		peerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := peer.NotifyFinalized(peerCtx, fb)
		cancel()
		if err != nil {
			log.Printf("consensus: notify peer %s failed: %v", peer.Addr, err)
		}
	}

	log.Printf("consensus: finalized block %d (txs=%d, voteStake=%d/%d)", height, len(txs), voteStake, required)
	return nil
}

// commitBlock persists a finalized block and advances state. Caller must hold mu.
func (e *Engine) commitBlock(block *types.Block, newState *state.WorldState) error {
	if err := e.BlockStore.SaveBlock(block); err != nil {
		return fmt.Errorf("save block: %w", err)
	}
	if err := e.KVStore.SaveState(newState); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	// Remove included txs from mempool.
	hashes := make([][32]byte, len(block.Transactions))
	for i, tx := range block.Transactions {
		hashes[i] = tx.Hash()
	}
	e.Mempool.Remove(hashes)

	// Evict stale txs (nonce already used or sender doesn't exist).
	e.Mempool.EvictStale(func(addr crypto.Address) (uint64, bool) {
		acct, ok := newState.GetAccount(addr)
		if !ok {
			return 0, false
		}
		return acct.Nonce, true
	})

	e.State = newState
	e.lastBlock = block
	e.currentHeight = block.Header.Height

	// Signal the Run loop that a block was committed.
	select {
	case e.blockCommitted <- struct{}{}:
	default:
	}

	return nil
}

// --- Handler methods (called by P2P server) ---

// OnProposal handles an incoming block proposal from the proposer.
func (e *Engine) OnProposal(ctx context.Context, proposal *pb.BlockProposal) (*pb.BlockVote, error) {
	block := p2p.BlockFromProto(proposal.Block)

	e.mu.Lock()
	defer e.mu.Unlock()

	err := ValidateProposedBlock(block, e.State, e.lastBlock, e.activeValidators(), e.Params)
	if err != nil {
		return nil, fmt.Errorf("invalid proposal: %w", err)
	}

	// Sign vote.
	blockHash := block.Header.Hash()
	vote := types.BlockVote{
		Height:    block.Header.Height,
		BlockHash: blockHash,
		VoterAddr: e.Address,
	}
	types.SignBlockVote(&vote, e.PrivKey)

	return p2p.VoteToProto(&vote), nil
}

// OnFinalizedBlock handles notification of a finalized block.
func (e *Engine) OnFinalizedBlock(ctx context.Context, fb *pb.FinalizedBlock) (*pb.PeerStatus, error) {
	block := p2p.BlockFromProto(fb.Block)
	qc := p2p.QCFromProto(fb.Qc)

	e.mu.Lock()
	defer e.mu.Unlock()

	// Verify QC.
	blockHash := block.Header.Hash()
	if err := ValidateQuorumCertificate(qc, blockHash, e.activeValidators()); err != nil {
		return nil, fmt.Errorf("invalid QC: %w", err)
	}

	// Skip if we already have this block.
	if block.Header.Height <= e.currentHeight {
		return e.statusProto(), nil
	}

	// Validate and apply.
	err := ValidateProposedBlock(block, e.State, e.lastBlock, e.activeValidators(), e.Params)
	if err != nil {
		return nil, fmt.Errorf("invalid finalized block: %w", err)
	}

	newState, err := e.Executor.ApplyBlock(e.State, block)
	if err != nil {
		return nil, fmt.Errorf("apply finalized block: %w", err)
	}

	block.QC = qc
	if err := e.commitBlock(block, newState); err != nil {
		return nil, err
	}

	log.Printf("consensus: committed finalized block %d from network", block.Header.Height)
	return e.statusProto(), nil
}

// OnSyncRequest handles a chain sync request.
func (e *Engine) OnSyncRequest(ctx context.Context, from, to uint64) (*pb.SyncResponse, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if to == 0 || to > e.currentHeight {
		to = e.currentHeight
	}
	if from > to {
		return &pb.SyncResponse{}, nil
	}

	// Cap at 100 blocks per request.
	if to-from+1 > 100 {
		to = from + 99
	}

	var blocks []*pb.FinalizedBlock
	for h := from; h <= to; h++ {
		block, err := e.BlockStore.GetBlockByHeight(h)
		if err != nil {
			break
		}
		fb := &pb.FinalizedBlock{
			Block: p2p.BlockToProto(block),
		}
		if block.QC != nil {
			fb.Qc = p2p.QCToProto(block.QC)
		}
		blocks = append(blocks, fb)
	}
	return &pb.SyncResponse{Blocks: blocks}, nil
}

// OnGetStatus returns this node's current status.
func (e *Engine) OnGetStatus(ctx context.Context) (*pb.PeerStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.statusProto(), nil
}

// OnSubmitTx handles an incoming transaction.
func (e *Engine) OnSubmitTx(ctx context.Context, sub *pb.TxSubmission) (*pb.TxSubmissionResponse, error) {
	tx := p2p.TxFromProto(sub.Tx)
	if err := e.Mempool.Add(tx); err != nil {
		return &pb.TxSubmissionResponse{Accepted: false, Error: err.Error()}, nil
	}

	// Relay to peers.
	for _, peer := range e.Peers.Peers() {
		go func(c *p2p.Client) {
			peerCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			c.SubmitTx(peerCtx, sub)
		}(peer)
	}

	return &pb.TxSubmissionResponse{Accepted: true}, nil
}

func (e *Engine) statusProto() *pb.PeerStatus {
	s := &pb.PeerStatus{
		Address:       e.Address[:],
		LatestHeight:  e.currentHeight,
		ChainId:       e.Params.ChainID,
	}
	if e.lastBlock != nil {
		h := e.lastBlock.Header.Hash()
		s.LatestBlockHash = h[:]
	}
	return s
}

// SyncToNetwork catches up to the highest peer by requesting blocks.
func (e *Engine) SyncToNetwork(ctx context.Context) error {
	peers := e.Peers.Peers()
	if len(peers) == 0 {
		return nil
	}

	// Find the highest peer.
	var bestPeer *p2p.Client
	var bestHeight uint64
	for _, peer := range peers {
		peerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		status, err := peer.GetStatus(peerCtx)
		cancel()
		if err != nil {
			continue
		}
		if status.LatestHeight > bestHeight {
			bestHeight = status.LatestHeight
			bestPeer = peer
		}
	}

	if bestPeer == nil || bestHeight <= e.currentHeight {
		return nil
	}

	log.Printf("consensus: syncing from height %d to %d", e.currentHeight+1, bestHeight)

	for e.currentHeight < bestHeight {
		from := e.currentHeight + 1
		to := bestHeight
		peerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		resp, err := bestPeer.SyncBlocks(peerCtx, from, to)
		cancel()
		if err != nil {
			return fmt.Errorf("sync blocks: %w", err)
		}
		if len(resp.Blocks) == 0 {
			break
		}

		e.mu.Lock()
		for _, fb := range resp.Blocks {
			block := p2p.BlockFromProto(fb.Block)
			if fb.Qc != nil {
				qc := p2p.QCFromProto(fb.Qc)
				blockHash := block.Header.Hash()
				if err := ValidateQuorumCertificate(qc, blockHash, e.activeValidators()); err != nil {
					e.mu.Unlock()
					return fmt.Errorf("sync: invalid QC at height %d: %w", block.Header.Height, err)
				}
				block.QC = qc
			}

			if err := ValidateProposedBlock(block, e.State, e.lastBlock, e.activeValidators(), e.Params); err != nil {
				e.mu.Unlock()
				return fmt.Errorf("sync: invalid block at height %d: %w", block.Header.Height, err)
			}

			newState, err := e.Executor.ApplyBlock(e.State, block)
			if err != nil {
				e.mu.Unlock()
				return fmt.Errorf("sync: apply block %d: %w", block.Header.Height, err)
			}

			if err := e.commitBlock(block, newState); err != nil {
				e.mu.Unlock()
				return fmt.Errorf("sync: commit block %d: %w", block.Header.Height, err)
			}
		}
		e.mu.Unlock()
	}

	log.Printf("consensus: sync complete at height %d", e.currentHeight)
	return nil
}
