package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/drana-chain/drana/internal/consensus"
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/p2p"
	pb "github.com/drana-chain/drana/internal/proto/pb"
	"github.com/drana-chain/drana/internal/store"
	"github.com/drana-chain/drana/internal/types"
)

// Server is the JSON HTTP RPC server for client-facing queries and tx submission.
type Server struct {
	engine     *consensus.Engine
	blockStore *store.BlockStore
	genesis    *types.GenesisConfig
	httpServer *http.Server
}

// NewServer creates a new RPC server.
func NewServer(
	listenAddr string,
	engine *consensus.Engine,
	blockStore *store.BlockStore,
	genesis *types.GenesisConfig,
) *Server {
	s := &Server{
		engine:     engine,
		blockStore: blockStore,
		genesis:    genesis,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/node/info", s.handleGetNodeInfo)
	mux.HandleFunc("/v1/blocks/latest", s.handleGetLatestBlock)
	mux.HandleFunc("/v1/blocks/hash/", s.handleGetBlockByHash)
	mux.HandleFunc("/v1/blocks/", s.handleGetBlockByHeight)
	mux.HandleFunc("/v1/accounts/name/", s.handleGetAccountByName)
	mux.HandleFunc("/v1/accounts/", s.handleAccountRoutes)
	mux.HandleFunc("/v1/posts/", s.handlePosts)
	mux.HandleFunc("/v1/posts", s.handlePosts)
	mux.HandleFunc("/v1/transactions/", s.handleTransactions)
	mux.HandleFunc("/v1/transactions", s.handleTransactions)
	mux.HandleFunc("/v1/mempool/pending", s.handleMempoolPending)
	mux.HandleFunc("/v1/network/validators", s.handleListValidators)
	mux.HandleFunc("/v1/network/peers", s.handleListPeers)
	registerDocs(mux)

	s.httpServer = &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	return s
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	log.Printf("rpc: listening on %s", s.httpServer.Addr)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("rpc: serve error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func txTypeString(t types.TxType) string {
	switch t {
	case types.TxTransfer:
		return "transfer"
	case types.TxCreatePost:
		return "create_post"
	case types.TxBoostPost:
		return "boost_post"
	case types.TxRegisterName:
		return "register_name"
	case types.TxStake:
		return "stake"
	case types.TxUnstake:
		return "unstake"
	case types.TxUnstakePost:
		return "unstake_post"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func parseTxType(s string) (types.TxType, error) {
	switch s {
	case "transfer":
		return types.TxTransfer, nil
	case "create_post":
		return types.TxCreatePost, nil
	case "boost_post":
		return types.TxBoostPost, nil
	case "register_name":
		return types.TxRegisterName, nil
	case "stake":
		return types.TxStake, nil
	case "unstake":
		return types.TxUnstake, nil
	case "unstake_post":
		return types.TxUnstakePost, nil
	default:
		return 0, fmt.Errorf("unknown tx type: %q", s)
	}
}

func blockToResponse(block *types.Block, includeTxs bool) BlockResponse {
	headerHash := block.Header.Hash()
	resp := BlockResponse{
		Height:       block.Header.Height,
		Hash:         hex.EncodeToString(headerHash[:]),
		PrevHash:     hex.EncodeToString(block.Header.PrevHash[:]),
		ProposerAddr: addrString(block.Header.ProposerAddr),
		Timestamp:    block.Header.Timestamp,
		StateRoot:    hex.EncodeToString(block.Header.StateRoot[:]),
		TxRoot:       hex.EncodeToString(block.Header.TxRoot[:]),
		TxCount:      len(block.Transactions),
	}
	if includeTxs {
		for _, tx := range block.Transactions {
			resp.Transactions = append(resp.Transactions, txToResponse(tx, block.Header.Height))
		}
	}
	return resp
}

func txToResponse(tx *types.Transaction, blockHeight uint64) TransactionResponse {
	txHash := tx.Hash()
	resp := TransactionResponse{
		Hash:        hex.EncodeToString(txHash[:]),
		Type:        txTypeString(tx.Type),
		Sender:      addrString(tx.Sender),
		Amount:      tx.Amount,
		Nonce:       tx.Nonce,
		BlockHeight: blockHeight,
	}
	var zeroPostID types.PostID
	switch tx.Type {
	case types.TxTransfer:
		resp.Recipient = addrString(tx.Recipient)
	case types.TxCreatePost:
		resp.Text = tx.Text
		resp.Channel = tx.Channel
		if tx.PostID != zeroPostID {
			resp.ParentPostID = hex.EncodeToString(tx.PostID[:])
		}
	case types.TxBoostPost:
		resp.PostID = hex.EncodeToString(tx.PostID[:])
	}
	return resp
}

func postToResponse(p *types.Post) PostResponse {
	var parentID string
	var zeroID types.PostID
	if p.ParentPostID != zeroID {
		parentID = hex.EncodeToString(p.ParentPostID[:])
	}
	return PostResponse{
		PostID:          hex.EncodeToString(p.PostID[:]),
		Author:          addrString(p.Author),
		Text:            p.Text,
		Channel:         p.Channel,
		ParentPostID:    parentID,
		CreatedAtHeight: p.CreatedAtHeight,
		CreatedAtTime:   p.CreatedAtTime,
		TotalStaked:     p.TotalStaked,
		TotalBurned:     p.TotalBurned,
		StakerCount:     p.StakerCount,
		Withdrawn:       p.Withdrawn,
	}
}

func addrString(a crypto.Address) string {
	return a.String()
}

// --- handlers ---

func (s *Server) handleGetNodeInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	ws := s.engine.CurrentState()
	height := s.engine.CurrentHeight()
	var latestHash string
	if b, err := s.blockStore.GetLatestBlock(); err == nil {
		h := b.Header.Hash()
		latestHash = hex.EncodeToString(h[:])
	}
	// Compute epoch info.
	epoch := ws.GetCurrentEpoch()
	epochLen := s.genesis.EpochLength
	var blocksUntilNext uint64
	if epochLen > 0 && height > 0 {
		blocksInCurrentEpoch := height % epochLen
		blocksUntilNext = epochLen - blocksInCurrentEpoch
	}

	writeJSON(w, 200, NodeInfoResponse{
		ChainID:              s.genesis.ChainID,
		LatestHeight:         height,
		LatestHash:           latestHash,
		GenesisTime:          s.genesis.GenesisTime,
		BlockInterval:        s.genesis.BlockIntervalSec,
		BlockReward:          s.genesis.BlockReward,
		BurnedSupply:         ws.GetBurnedSupply(),
		IssuedSupply:         ws.GetIssuedSupply(),
		ValidatorCount:       len(ws.GetActiveValidators()),
		CurrentEpoch:         epoch,
		EpochLength:          epochLen,
		BlocksUntilNextEpoch: blocksUntilNext,
	})
}

func (s *Server) handleGetLatestBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	block, err := s.blockStore.GetLatestBlock()
	if err != nil {
		writeError(w, 404, "no blocks yet")
		return
	}
	full := r.URL.Query().Get("full") == "true"
	writeJSON(w, 200, blockToResponse(block, full))
}

func (s *Server) handleGetBlockByHeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	// Path: /v1/blocks/{height}
	heightStr := strings.TrimPrefix(r.URL.Path, "/v1/blocks/")
	if heightStr == "" || heightStr == "latest" || strings.HasPrefix(heightStr, "hash/") {
		return // handled by other routes
	}
	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid height")
		return
	}
	block, err := s.blockStore.GetBlockByHeight(height)
	if err != nil {
		writeError(w, 404, fmt.Sprintf("block at height %d not found", height))
		return
	}
	full := r.URL.Query().Get("full") == "true"
	writeJSON(w, 200, blockToResponse(block, full))
}

func (s *Server) handleGetBlockByHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	hashStr := strings.TrimPrefix(r.URL.Path, "/v1/blocks/hash/")
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil || len(hashBytes) != 32 {
		writeError(w, 400, "invalid block hash")
		return
	}
	var hash [32]byte
	copy(hash[:], hashBytes)
	block, err := s.blockStore.GetBlockByHash(hash)
	if err != nil {
		writeError(w, 404, "block not found")
		return
	}
	full := r.URL.Query().Get("full") == "true"
	writeJSON(w, 200, blockToResponse(block, full))
}

func (s *Server) handleAccountRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/accounts/")
	if strings.HasSuffix(path, "/unbonding") {
		addrStr := strings.TrimSuffix(path, "/unbonding")
		s.handleGetUnbonding(w, r, addrStr)
		return
	}
	if strings.HasSuffix(path, "/post-stakes") {
		addrStr := strings.TrimSuffix(path, "/post-stakes")
		s.handleGetPostStakes(w, r, addrStr)
		return
	}
	s.handleGetAccount(w, r)
}

func (s *Server) handleGetPostStakes(w http.ResponseWriter, r *http.Request, addrStr string) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	addr, err := crypto.ParseAddress(addrStr)
	if err != nil {
		writeError(w, 400, fmt.Sprintf("invalid address: %v", err))
		return
	}
	ws := s.engine.CurrentState()
	stakes := ws.GetStakesByAddress(addr)
	type stakeResp struct {
		PostID string `json:"postId"`
		Amount uint64 `json:"amount,string"`
		Height uint64 `json:"height,string"`
	}
	type stakesResponse struct {
		Stakes      []stakeResp `json:"stakes"`
		TotalCount  int         `json:"totalCount"`
		TotalStaked uint64      `json:"totalStaked,string"`
	}
	var items []stakeResp
	var totalStaked uint64
	for _, ps := range stakes {
		items = append(items, stakeResp{
			PostID: hex.EncodeToString(ps.PostID[:]),
			Amount: ps.Amount,
			Height: ps.Height,
		})
		totalStaked += ps.Amount
	}
	if items == nil {
		items = []stakeResp{}
	}
	writeJSON(w, 200, stakesResponse{
		Stakes: items, TotalCount: len(items), TotalStaked: totalStaked,
	})
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	addrStr := strings.TrimPrefix(r.URL.Path, "/v1/accounts/")
	addr, err := crypto.ParseAddress(addrStr)
	if err != nil {
		writeError(w, 400, fmt.Sprintf("invalid address: %v", err))
		return
	}
	ws := s.engine.CurrentState()
	acct, ok := ws.GetAccount(addr)
	if !ok {
		// Unknown address — return zero balance, not 404.
		writeJSON(w, 200, AccountResponse{
			Address: addr.String(),
			Balance: 0,
			Nonce:   0,
		})
		return
	}
	writeJSON(w, 200, AccountResponse{
		Address:       acct.Address.String(),
		Balance:       acct.Balance,
		Nonce:         acct.Nonce,
		Name:          acct.Name,
		StakedBalance:    acct.StakedBalance,
		PostStakeBalance: acct.PostStakeBalance,
	})
}

func (s *Server) handleGetUnbonding(w http.ResponseWriter, r *http.Request, addrStr string) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	addr, err := crypto.ParseAddress(addrStr)
	if err != nil {
		writeError(w, 400, fmt.Sprintf("invalid address: %v", err))
		return
	}
	ws := s.engine.CurrentState()
	queue := ws.GetUnbondingQueue()
	var entries []UnbondingEntryResponse
	var total uint64
	for _, e := range queue {
		if e.Address == addr {
			entries = append(entries, UnbondingEntryResponse{
				Amount:        e.Amount,
				ReleaseHeight: e.ReleaseHeight,
			})
			total += e.Amount
		}
	}
	if entries == nil {
		entries = []UnbondingEntryResponse{}
	}
	writeJSON(w, 200, UnbondingResponse{
		Address: addr.String(),
		Entries: entries,
		Total:   total,
	})
}

func (s *Server) handleGetAccountByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/v1/accounts/name/")
	if name == "" {
		writeError(w, 400, "missing name")
		return
	}
	ws := s.engine.CurrentState()
	acct, ok := ws.GetAccountByName(name)
	if !ok {
		writeError(w, 404, fmt.Sprintf("name %q not found", name))
		return
	}
	writeJSON(w, 200, AccountResponse{
		Address:       acct.Address.String(),
		Balance:       acct.Balance,
		Nonce:         acct.Nonce,
		Name:          acct.Name,
		StakedBalance:    acct.StakedBalance,
		PostStakeBalance: acct.PostStakeBalance,
	})
}

func (s *Server) handlePosts(w http.ResponseWriter, r *http.Request) {
	// Dispatch: /v1/posts vs /v1/posts/{id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/posts")
	if path == "" || path == "/" {
		if r.Method == http.MethodGet {
			s.handleListPosts(w, r)
			return
		}
		writeError(w, 405, "method not allowed")
		return
	}
	// /v1/posts/{id}
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	idStr := strings.TrimPrefix(path, "/")
	idBytes, err := hex.DecodeString(idStr)
	if err != nil || len(idBytes) != 32 {
		writeError(w, 400, "invalid post ID")
		return
	}
	var postID types.PostID
	copy(postID[:], idBytes)
	ws := s.engine.CurrentState()
	post, ok := ws.GetPost(postID)
	if !ok {
		writeError(w, 404, "post not found")
		return
	}
	writeJSON(w, 200, postToResponse(post))
}

func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	authorFilter := q.Get("author")
	channelFilter := q.Get("channel")
	includeReplies := q.Get("includeReplies") == "true"

	ws := s.engine.CurrentState()
	allPosts := ws.AllPosts()

	var zeroPostID types.PostID
	var filtered []*types.Post
	for _, p := range allPosts {
		// Exclude replies by default.
		if !includeReplies && p.ParentPostID != zeroPostID {
			continue
		}
		// Filter by author.
		if authorFilter != "" {
			addr, err := crypto.ParseAddress(authorFilter)
			if err != nil {
				writeError(w, 400, fmt.Sprintf("invalid author address: %v", err))
				return
			}
			if p.Author != addr {
				continue
			}
		}
		// Filter by channel.
		if channelFilter != "" && p.Channel != channelFilter {
			continue
		}
		filtered = append(filtered, p)
	}

	// Sort by creation height descending.
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAtHeight > filtered[j].CreatedAtHeight
	})

	totalCount := len(filtered)
	start := (page - 1) * pageSize
	if start >= totalCount {
		writeJSON(w, 200, PostListResponse{
			Posts:      []PostResponse{},
			TotalCount: totalCount,
			Page:       page,
			PageSize:   pageSize,
		})
		return
	}
	end := start + pageSize
	if end > totalCount {
		end = totalCount
	}

	posts := make([]PostResponse, 0, end-start)
	for _, p := range filtered[start:end] {
		posts = append(posts, postToResponse(p))
	}
	writeJSON(w, 200, PostListResponse{
		Posts:      posts,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	})
}

func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/transactions")

	// POST /v1/transactions — submit
	if (path == "" || path == "/") && r.Method == http.MethodPost {
		s.handleSubmitTransaction(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}

	// GET /v1/transactions/{hash}/status
	if strings.HasSuffix(path, "/status") {
		hashStr := strings.TrimPrefix(path, "/")
		hashStr = strings.TrimSuffix(hashStr, "/status")
		s.handleGetTransactionStatus(w, r, hashStr)
		return
	}

	// GET /v1/transactions/{hash}
	hashStr := strings.TrimPrefix(path, "/")
	s.handleGetTransaction(w, r, hashStr)
}

func (s *Server) handleSubmitTransaction(w http.ResponseWriter, r *http.Request) {
	var req SubmitTxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	txType, err := parseTxType(req.Type)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	tx := &types.Transaction{Type: txType, Amount: req.Amount, Nonce: req.Nonce, Text: req.Text, Channel: req.Channel}

	// Parse sender.
	sender, err := crypto.ParseAddress(req.Sender)
	if err != nil {
		writeError(w, 400, fmt.Sprintf("invalid sender: %v", err))
		return
	}
	tx.Sender = sender

	// Parse optional fields.
	if req.Recipient != "" {
		recip, err := crypto.ParseAddress(req.Recipient)
		if err != nil {
			writeError(w, 400, fmt.Sprintf("invalid recipient: %v", err))
			return
		}
		tx.Recipient = recip
	}
	if req.PostID != "" {
		pidBytes, err := hex.DecodeString(req.PostID)
		if err != nil || len(pidBytes) != 32 {
			writeError(w, 400, "invalid postId")
			return
		}
		copy(tx.PostID[:], pidBytes)
	}
	if req.ParentPostID != "" {
		pidBytes, err := hex.DecodeString(req.ParentPostID)
		if err != nil || len(pidBytes) != 32 {
			writeError(w, 400, "invalid parentPostId")
			return
		}
		copy(tx.PostID[:], pidBytes)
	}

	// Parse signature and pubkey.
	sig, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeError(w, 400, "invalid signature hex")
		return
	}
	tx.Signature = sig

	pkBytes, err := hex.DecodeString(req.PubKey)
	if err != nil || len(pkBytes) != 32 {
		writeError(w, 400, "invalid pubKey")
		return
	}
	copy(tx.PubKey[:], pkBytes)

	// Submit via engine.
	pbTx := p2p.TxToProto(tx)
	ctx := r.Context()
	resp, err := s.engine.OnSubmitTx(ctx, &pb.TxSubmission{Tx: pbTx})
	if err != nil {
		writeError(w, 500, fmt.Sprintf("submit failed: %v", err))
		return
	}

	txHash := tx.Hash()
	writeJSON(w, 200, SubmitTxResponse{
		Accepted: resp.Accepted,
		TxHash:   hex.EncodeToString(txHash[:]),
		Error:    resp.Error,
	})
}

func (s *Server) handleGetTransaction(w http.ResponseWriter, r *http.Request, hashStr string) {
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil || len(hashBytes) != 32 {
		writeError(w, 400, "invalid transaction hash")
		return
	}
	var txHash [32]byte
	copy(txHash[:], hashBytes)

	tx, blockHeight, err := s.blockStore.GetTransaction(txHash)
	if err != nil {
		writeError(w, 404, "transaction not found")
		return
	}
	writeJSON(w, 200, txToResponse(tx, blockHeight))
}

func (s *Server) handleGetTransactionStatus(w http.ResponseWriter, r *http.Request, hashStr string) {
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil || len(hashBytes) != 32 {
		writeError(w, 400, "invalid transaction hash")
		return
	}
	var txHash [32]byte
	copy(txHash[:], hashBytes)

	// Check confirmed.
	_, blockHeight, err := s.blockStore.GetTransaction(txHash)
	if err == nil {
		writeJSON(w, 200, TxStatusResponse{
			Hash:        hashStr,
			Status:      "confirmed",
			BlockHeight: blockHeight,
		})
		return
	}

	// Check pending.
	if s.engine.Mempool.Has(txHash) {
		writeJSON(w, 200, TxStatusResponse{Hash: hashStr, Status: "pending"})
		return
	}

	writeJSON(w, 200, TxStatusResponse{Hash: hashStr, Status: "unknown"})
}

func (s *Server) handleMempoolPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	senderFilter := r.URL.Query().Get("sender")
	var senderAddr crypto.Address
	var filterBySender bool
	if senderFilter != "" {
		addr, err := crypto.ParseAddress(senderFilter)
		if err != nil {
			writeError(w, 400, fmt.Sprintf("invalid sender: %v", err))
			return
		}
		senderAddr = addr
		filterBySender = true
	}

	pending := s.engine.Mempool.Pending()
	var txs []TransactionResponse
	for _, tx := range pending {
		if filterBySender && tx.Sender != senderAddr {
			continue
		}
		txs = append(txs, txToResponse(tx, 0))
		if len(txs) >= 100 {
			break
		}
	}
	if txs == nil {
		txs = []TransactionResponse{}
	}
	writeJSON(w, 200, MempoolResponse{Transactions: txs, Count: len(txs)})
}

func (s *Server) handleListValidators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	ws := s.engine.CurrentState()
	activeSet := ws.GetActiveValidators()
	vals := make([]ValidatorResponse, len(activeSet))
	for i, v := range activeSet {
		name := ""
		if acct, ok := ws.GetAccount(v.Address); ok {
			name = acct.Name
		}
		vals[i] = ValidatorResponse{
			Address:       v.Address.String(),
			Name:          name,
			PubKey:        hex.EncodeToString(v.PubKey[:]),
			StakedBalance: v.StakedBalance,
		}
	}
	writeJSON(w, 200, vals)
}

func (s *Server) handleListPeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	peers := s.engine.Peers.Peers()
	resp := make([]PeerResponse, len(peers))
	for i, p := range peers {
		resp[i] = PeerResponse{Endpoint: p.Addr}
	}
	writeJSON(w, 200, resp)
}
