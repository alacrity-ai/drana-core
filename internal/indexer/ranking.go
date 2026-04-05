package indexer

import (
	"fmt"
	"math"
)

type RankingStrategy string

const (
	RankByTrending      RankingStrategy = "trending"
	RankByTopAllTime    RankingStrategy = "top"
	RankByNew           RankingStrategy = "new"
	RankByControversial RankingStrategy = "controversial"
)

const trendingAlpha = 1.5
const trendingWindowHours = 72

func (d *DB) RankedPosts(strategy RankingStrategy, page, pageSize int, nowUnix int64, channel string) ([]RankedPost, int, error) {
	return d.rankedPostsQuery(strategy, "", channel, page, pageSize, nowUnix)
}

func (d *DB) RankedPostsByAuthor(author string, strategy RankingStrategy, page, pageSize int, nowUnix int64) ([]RankedPost, int, error) {
	return d.rankedPostsQuery(strategy, author, "", page, pageSize, nowUnix)
}

func (d *DB) rankedPostsQuery(strategy RankingStrategy, author, channel string, page, pageSize int, nowUnix int64) ([]RankedPost, int, error) {
	switch strategy {
	case RankByTopAllTime:
		return d.sqlPaginatedQuery("total_staked", "CAST(total_staked AS REAL)", author, channel, page, pageSize)
	case RankByNew:
		return d.sqlPaginatedQuery("created_at_height", "CAST(created_at_height AS REAL)", author, channel, page, pageSize)
	case RankByControversial:
		return d.sqlPaginatedQuery("(staker_count * 1.0) * total_staked", "(staker_count * 1.0) * total_staked", author, channel, page, pageSize)
	case RankByTrending:
		return d.trendingQuery(author, channel, page, pageSize, nowUnix)
	default:
		return d.sqlPaginatedQuery("total_staked", "CAST(total_staked AS REAL)", author, channel, page, pageSize)
	}
}

// sqlPaginatedQuery uses SQL ORDER BY + LIMIT/OFFSET for strategies that don't need Go-side computation.
func (d *DB) sqlPaginatedQuery(orderByExpr, scoreExpr, author, channel string, page, pageSize int) ([]RankedPost, int, error) {
	where, args := buildWhere(author, channel)

	var total int
	d.queryRow("SELECT COUNT(*) FROM posts "+where, args...).Scan(&total)

	offset := (page - 1) * pageSize

	cols := postCols + fmt.Sprintf(", %s as score", scoreExpr)
	queryArgs := append(append([]interface{}{}, args...), pageSize, offset)
	q := fmt.Sprintf("SELECT %s FROM posts %s ORDER BY %s DESC LIMIT ? OFFSET ?", cols, where, orderByExpr)

	rows, err := d.query(q, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query posts: %w", err)
	}
	defer rows.Close()

	var results []RankedPost
	for rows.Next() {
		var p IndexedPost
		var score float64
		var withdrawn int
		rows.Scan(&p.PostID, &p.Author, &p.Text, &p.Channel, &p.ParentPostID,
			&p.CreatedAtHeight, &p.CreatedAtTime,
			&p.TotalStaked, &p.AuthorStaked, &p.ThirdPartyStaked,
			&p.StakerCount, &p.UniqueBoosterCount, &p.LastBoostAtHeight, &p.ReplyCount,
			&withdrawn, &p.TotalBurned,
			&score)
		p.Withdrawn = withdrawn != 0
		results = append(results, RankedPost{IndexedPost: p, Score: score})
	}
	if results == nil {
		results = []RankedPost{}
	}
	return results, total, nil
}

// trendingQuery fetches only recent posts (last 72h), scores in Go, sorts, and slices.
func (d *DB) trendingQuery(author, channel string, page, pageSize int, nowUnix int64) ([]RankedPost, int, error) {
	where, args := buildWhere(author, channel)

	// Add time window filter for the data query (not the count — count is all top-level posts).
	var total int
	d.queryRow("SELECT COUNT(*) FROM posts "+where, args...).Scan(&total)

	cutoff := nowUnix - int64(trendingWindowHours*3600)
	trendingWhere := where + " AND created_at_time > ?"
	trendingArgs := append(append([]interface{}{}, args...), cutoff)

	rows, err := d.query("SELECT "+postCols+" FROM posts "+trendingWhere, trendingArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query trending: %w", err)
	}
	defer rows.Close()

	var all []RankedPost
	for rows.Next() {
		var p IndexedPost
		var withdrawn int
		rows.Scan(&p.PostID, &p.Author, &p.Text, &p.Channel, &p.ParentPostID,
			&p.CreatedAtHeight, &p.CreatedAtTime,
			&p.TotalStaked, &p.AuthorStaked, &p.ThirdPartyStaked,
			&p.StakerCount, &p.UniqueBoosterCount, &p.LastBoostAtHeight, &p.ReplyCount,
			&withdrawn, &p.TotalBurned)
		p.Withdrawn = withdrawn != 0
		score := trendingScore(&p, nowUnix)
		all = append(all, RankedPost{IndexedPost: p, Score: score})
	}

	// Sort by score descending.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].Score > all[j-1].Score; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}

	// Paginate.
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []RankedPost{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

func trendingScore(p *IndexedPost, nowUnix int64) float64 {
	ageHours := float64(nowUnix-p.CreatedAtTime) / 3600.0
	if ageHours < 0 {
		ageHours = 0
	}
	return math.Log1p(float64(p.TotalStaked)) / math.Pow(1+ageHours, trendingAlpha)
}

// --- helpers ---

const postCols = `post_id, author, text, channel, parent_post_id,
	created_at_height, created_at_time,
	total_staked, author_staked, third_party_staked,
	staker_count, unique_booster_count, last_boost_at_height, reply_count,
	withdrawn, total_burned`

func buildWhere(author, channel string) (string, []interface{}) {
	where := "WHERE parent_post_id = ''"
	var args []interface{}
	if author != "" {
		where += " AND author = ?"
		args = append(args, author)
	}
	if channel != "" {
		where += " AND channel = ?"
		args = append(args, channel)
	}
	return where, args
}
