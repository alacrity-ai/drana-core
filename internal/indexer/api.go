package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIServer is the indexer's HTTP query API.
type APIServer struct {
	db         *DB
	httpServer *http.Server
	// nodeRPC is used to fetch supply stats for /v1/stats.
	nodeRPC string
}

// NewAPIServer creates a new indexer API server.
func NewAPIServer(listenAddr string, db *DB, nodeRPC string) *APIServer {
	a := &APIServer{db: db, nodeRPC: nodeRPC}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/feed/author/", a.handleFeedByAuthor)
	mux.HandleFunc("/v1/feed", a.handleFeed)
	mux.HandleFunc("/v1/channels", a.handleChannels)
	mux.HandleFunc("/v1/posts/", a.handlePost)
	mux.HandleFunc("/v1/authors/", a.handleAuthor)
	mux.HandleFunc("/v1/rewards/", a.handleRewards)
	mux.HandleFunc("/v1/stats", a.handleStats)
	mux.HandleFunc("/v1/leaderboard", a.handleLeaderboard)
	registerDocs(mux)

	a.httpServer = &http.Server{Addr: listenAddr, Handler: mux}
	return a
}

func (a *APIServer) Start() error {
	log.Printf("indexer api: listening on %s", a.httpServer.Addr)
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("indexer api: serve error: %v", err)
		}
	}()
	return nil
}

func (a *APIServer) Stop(ctx context.Context) error {
	return a.httpServer.Shutdown(ctx)
}

// --- response types ---

type FeedResponse struct {
	Posts      []RankedPost `json:"posts"`
	TotalCount int          `json:"totalCount"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	Strategy   string       `json:"strategy"`
}

type BoostHistoryResponse struct {
	Boosts     []IndexedBoost `json:"boosts"`
	TotalCount int            `json:"totalCount"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
}

type StatsResponse struct {
	LatestHeight      uint64 `json:"latestHeight"`
	TotalPosts        int    `json:"totalPosts"`
	TotalBoosts       int    `json:"totalBoosts"`
	TotalTransfers    int    `json:"totalTransfers"`
	TotalBurned       uint64 `json:"totalBurned"`
	TotalIssued       uint64 `json:"totalIssued"`
	CirculatingSupply uint64 `json:"circulatingSupply"`
}

type LeaderboardResponse struct {
	Authors    []LeaderboardEntry `json:"authors"`
	TotalCount int                `json:"totalCount"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
}

type ReplyListResponse struct {
	Replies    []IndexedPost `json:"replies"`
	TotalCount int           `json:"totalCount"`
	Page       int           `json:"page"`
	PageSize   int           `json:"pageSize"`
}

type RewardSummaryResponse struct {
	Last24h     uint64 `json:"last24h,string"`
	Last7d      uint64 `json:"last7d,string"`
	AllTime     uint64 `json:"allTime,string"`
	PostCount   int    `json:"postCount"`
	TotalStaked uint64 `json:"totalStaked,string"`
}

type RewardPostResponse struct {
	TotalReward uint64 `json:"totalReward,string"`
}

type RewardEventListResponse struct {
	Events      []RewardEvent `json:"events"`
	TotalCount  int           `json:"totalCount"`
	TotalAmount uint64        `json:"totalAmount,string"`
}

type apiError struct {
	Error string `json:"error"`
}

// --- helpers ---

func apiWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func apiWriteError(w http.ResponseWriter, status int, msg string) {
	apiWriteJSON(w, status, apiError{Error: msg})
}

func pageParams(r *http.Request) (int, int) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func strategyParam(r *http.Request) RankingStrategy {
	s := r.URL.Query().Get("strategy")
	switch RankingStrategy(s) {
	case RankByTrending, RankByTopAllTime, RankByNew, RankByControversial:
		return RankingStrategy(s)
	default:
		return RankByTrending
	}
}

// --- handlers ---

func (a *APIServer) handleFeed(w http.ResponseWriter, r *http.Request) {
	page, pageSize := pageParams(r)
	strategy := strategyParam(r)
	channel := r.URL.Query().Get("channel")
	now := time.Now().Unix()

	posts, total, err := a.db.RankedPosts(strategy, page, pageSize, now, channel)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	apiWriteJSON(w, 200, FeedResponse{
		Posts: posts, TotalCount: total, Page: page, PageSize: pageSize, Strategy: string(strategy),
	})
}

func (a *APIServer) handleFeedByAuthor(w http.ResponseWriter, r *http.Request) {
	author := strings.TrimPrefix(r.URL.Path, "/v1/feed/author/")
	if author == "" {
		apiWriteError(w, 400, "missing author address")
		return
	}
	page, pageSize := pageParams(r)
	strategy := strategyParam(r)
	now := time.Now().Unix()

	posts, total, err := a.db.RankedPostsByAuthor(author, strategy, page, pageSize, now)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	apiWriteJSON(w, 200, FeedResponse{
		Posts: posts, TotalCount: total, Page: page, PageSize: pageSize, Strategy: string(strategy),
	})
}

func (a *APIServer) handleChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := a.db.GetChannels()
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	if channels == nil {
		channels = []ChannelInfo{}
	}
	apiWriteJSON(w, 200, channels)
}

func (a *APIServer) handlePost(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/posts/")
	// /v1/posts/{id}/boosts
	if strings.HasSuffix(path, "/boosts") {
		postID := strings.TrimSuffix(path, "/boosts")
		a.handleBoostHistory(w, r, postID)
		return
	}
	// /v1/posts/{id}/replies
	if strings.HasSuffix(path, "/replies") {
		postID := strings.TrimSuffix(path, "/replies")
		page, pageSize := pageParams(r)
		replies, total, err := a.db.GetReplies(postID, page, pageSize)
		if err != nil {
			apiWriteError(w, 500, err.Error())
			return
		}
		if replies == nil {
			replies = []IndexedPost{}
		}
		apiWriteJSON(w, 200, ReplyListResponse{
			Replies: replies, TotalCount: total, Page: page, PageSize: pageSize,
		})
		return
	}

	postID := path
	if postID == "" {
		apiWriteError(w, 400, "missing post ID")
		return
	}

	post, err := a.db.GetPost(postID)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	if post == nil {
		apiWriteError(w, 404, "post not found")
		return
	}
	apiWriteJSON(w, 200, post)
}

func (a *APIServer) handleBoostHistory(w http.ResponseWriter, r *http.Request, postID string) {
	page, pageSize := pageParams(r)
	boosts, total, err := a.db.GetBoostsForPost(postID, page, pageSize)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	if boosts == nil {
		boosts = []IndexedBoost{}
	}
	apiWriteJSON(w, 200, BoostHistoryResponse{
		Boosts: boosts, TotalCount: total, Page: page, PageSize: pageSize,
	})
}

func (a *APIServer) handleAuthor(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimPrefix(r.URL.Path, "/v1/authors/")
	if address == "" {
		apiWriteError(w, 400, "missing address")
		return
	}

	profile, err := a.db.GetAuthorProfile(address)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	if profile == nil {
		apiWriteError(w, 404, "author not found")
		return
	}
	apiWriteJSON(w, 200, profile)
}

func (a *APIServer) handleRewards(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/rewards/")
	if path == "" {
		apiWriteError(w, 400, "missing address")
		return
	}

	// /v1/rewards/{addr}/summary
	if strings.HasSuffix(path, "/summary") {
		addr := strings.TrimSuffix(path, "/summary")
		now := time.Now().Unix()
		last24h, last7d, allTime, err := a.db.GetRewardSummary(addr, now)
		if err != nil {
			apiWriteError(w, 500, err.Error())
			return
		}
		stakes, totalStaked := a.db.GetStakesByAddress(addr)
		apiWriteJSON(w, 200, RewardSummaryResponse{
			Last24h: last24h, Last7d: last7d, AllTime: allTime,
			PostCount: len(stakes), TotalStaked: totalStaked,
		})
		return
	}

	// /v1/rewards/{addr}/post/{postId}
	if strings.Contains(path, "/post/") {
		parts := strings.SplitN(path, "/post/", 2)
		if len(parts) != 2 || parts[1] == "" {
			apiWriteError(w, 400, "missing post ID")
			return
		}
		total, err := a.db.GetRewardsForPost(parts[0], parts[1])
		if err != nil {
			apiWriteError(w, 500, err.Error())
			return
		}
		apiWriteJSON(w, 200, RewardPostResponse{TotalReward: total})
		return
	}

	// /v1/rewards/{addr} — paginated event list
	addr := path
	page, pageSize := pageParams(r)
	sinceHeight := uint64(0)
	if s := r.URL.Query().Get("since"); s != "" {
		fmt.Sscanf(s, "%d", &sinceHeight)
	}

	events, total, totalAmount, err := a.db.GetRewardsByAddress(addr, sinceHeight, page, pageSize)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	if events == nil {
		events = []RewardEvent{}
	}
	apiWriteJSON(w, 200, RewardEventListResponse{
		Events: events, TotalCount: total, TotalAmount: totalAmount,
	})
}

func (a *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	lastH, _ := a.db.GetLastIndexedHeight()

	// Fetch supply info from node.
	var totalBurned, totalIssued uint64
	if a.nodeRPC != "" {
		type nodeInfo struct {
			BurnedSupply uint64 `json:"burnedSupply"`
			IssuedSupply uint64 `json:"issuedSupply"`
		}
		var info nodeInfo
		if err := httpGetJSON(a.nodeRPC+"/v1/node/info", &info); err == nil {
			totalBurned = info.BurnedSupply
			totalIssued = info.IssuedSupply
		}
	}

	stats := a.db.GetChainStats(lastH, totalBurned, totalIssued)
	apiWriteJSON(w, 200, StatsResponse{
		LatestHeight:      stats.LatestHeight,
		TotalPosts:        stats.TotalPosts,
		TotalBoosts:       stats.TotalBoosts,
		TotalTransfers:    stats.TotalTransfers,
		TotalBurned:       stats.TotalBurned,
		TotalIssued:       stats.TotalIssued,
		CirculatingSupply: stats.CirculatingSupply,
	})
}

func (a *APIServer) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	page, pageSize := pageParams(r)
	entries, total, err := a.db.GetLeaderboard(page, pageSize)
	if err != nil {
		apiWriteError(w, 500, err.Error())
		return
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	apiWriteJSON(w, 200, LeaderboardResponse{
		Authors: entries, TotalCount: total, Page: page, PageSize: pageSize,
	})
}

// Satisfy the compiler — nodeRPC supply fetch helper.
func init() {
	_ = fmt.Sprintf
}
