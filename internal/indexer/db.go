package indexer

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// DB wraps a database connection for the indexer (SQLite or PostgreSQL).
type DB struct {
	db     *sql.DB
	driver string // "sqlite" or "pgx"
}

// OpenDB opens a database. If dsn starts with "postgres://" or "postgresql://",
// it uses PostgreSQL via pgx. Otherwise it uses SQLite.
func OpenDB(dsn string) (*DB, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		return &DB{db: db, driver: "pgx"}, nil
	}
	// SQLite
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	return &DB{db: db, driver: "sqlite"}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

// rewrite converts ? placeholders to $1,$2,... for Postgres.
func (d *DB) rewrite(query string) string {
	if d.driver != "pgx" {
		return query
	}
	out := make([]byte, 0, len(query)+32)
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out = append(out, '$')
			out = append(out, []byte(fmt.Sprintf("%d", n))...)
			n++
		} else {
			out = append(out, query[i])
		}
	}
	return string(out)
}

// exec wraps db.Exec with placeholder rewriting.
func (d *DB) exec(query string, args ...interface{}) (sql.Result, error) {
	return d.db.Exec(d.rewrite(query), args...)
}

// queryRow wraps db.QueryRow with placeholder rewriting.
func (d *DB) queryRow(query string, args ...interface{}) *sql.Row {
	return d.db.QueryRow(d.rewrite(query), args...)
}

// query wraps db.Query with placeholder rewriting.
func (d *DB) query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.Query(d.rewrite(query), args...)
}

// boolToInt converts a bool to 0/1 for SQL storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// insertIgnore returns the dialect-appropriate INSERT-or-skip prefix.
func (d *DB) insertIgnore() string {
	if d.driver == "pgx" {
		return "INSERT INTO"
	}
	return "INSERT OR IGNORE INTO"
}

// onConflictIgnore returns the dialect-appropriate conflict clause suffix.
func (d *DB) onConflictIgnore() string {
	if d.driver == "pgx" {
		return " ON CONFLICT DO NOTHING"
	}
	return ""
}

// Migrate creates tables if they don't exist.
func (d *DB) Migrate() error {
	// Postgres INTEGER is 32-bit (max ~2.1B) — microdrana amounts can exceed that.
	// SQLite INTEGER is already 64-bit. Use BIGINT for Postgres amount/height/time columns.
	intType := "INTEGER"
	if d.driver == "pgx" {
		intType = "BIGINT"
	}

	schema := `
	CREATE TABLE IF NOT EXISTS posts (
		post_id TEXT PRIMARY KEY,
		author TEXT NOT NULL,
		text TEXT NOT NULL,
		channel TEXT NOT NULL DEFAULT '',
		parent_post_id TEXT NOT NULL DEFAULT '',
		created_at_height ` + intType + ` NOT NULL,
		created_at_time ` + intType + ` NOT NULL,
		total_staked ` + intType + ` NOT NULL DEFAULT 0,
		author_staked ` + intType + ` NOT NULL DEFAULT 0,
		third_party_staked ` + intType + ` NOT NULL DEFAULT 0,
		staker_count ` + intType + ` NOT NULL DEFAULT 0,
		unique_booster_count INTEGER NOT NULL DEFAULT 0,
		last_boost_at_height ` + intType + ` NOT NULL DEFAULT 0,
		reply_count INTEGER NOT NULL DEFAULT 0,
		withdrawn INTEGER NOT NULL DEFAULT 0,
		total_burned ` + intType + ` NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_posts_author ON posts(author);
	CREATE INDEX IF NOT EXISTS idx_posts_height ON posts(created_at_height);
	CREATE INDEX IF NOT EXISTS idx_posts_committed ON posts(total_staked);
	CREATE INDEX IF NOT EXISTS idx_posts_channel ON posts(channel);
	CREATE INDEX IF NOT EXISTS idx_posts_parent ON posts(parent_post_id);
	CREATE INDEX IF NOT EXISTS idx_posts_staker_count ON posts(staker_count);
	CREATE INDEX IF NOT EXISTS idx_posts_created_time ON posts(created_at_time);

	CREATE TABLE IF NOT EXISTS boosts (
		tx_hash TEXT PRIMARY KEY,
		post_id TEXT NOT NULL,
		booster TEXT NOT NULL,
		amount ` + intType + ` NOT NULL,
		author_reward ` + intType + ` NOT NULL DEFAULT 0,
		staker_reward ` + intType + ` NOT NULL DEFAULT 0,
		burn_amount ` + intType + ` NOT NULL DEFAULT 0,
		staked_amount ` + intType + ` NOT NULL DEFAULT 0,
		block_height ` + intType + ` NOT NULL,
		block_time ` + intType + ` NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_boosts_post ON boosts(post_id);
	CREATE INDEX IF NOT EXISTS idx_boosts_booster ON boosts(booster);

	CREATE TABLE IF NOT EXISTS transfers (
		tx_hash TEXT PRIMARY KEY,
		sender TEXT NOT NULL,
		recipient TEXT NOT NULL,
		amount ` + intType + ` NOT NULL,
		block_height ` + intType + ` NOT NULL,
		block_time ` + intType + ` NOT NULL
	);

	CREATE TABLE IF NOT EXISTS reward_events (
		id ` + d.autoIncPK() + `,
		post_id TEXT NOT NULL,
		recipient TEXT NOT NULL,
		amount ` + intType + ` NOT NULL,
		block_height ` + intType + ` NOT NULL,
		block_time ` + intType + ` NOT NULL,
		trigger_tx TEXT NOT NULL,
		trigger_address TEXT NOT NULL,
		reward_type TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_rewards_recipient ON reward_events(recipient);
	CREATE INDEX IF NOT EXISTS idx_rewards_post ON reward_events(post_id);
	CREATE INDEX IF NOT EXISTS idx_rewards_height ON reward_events(block_height);

	CREATE TABLE IF NOT EXISTS post_stakes (
		post_id TEXT NOT NULL,
		staker TEXT NOT NULL,
		amount ` + intType + ` NOT NULL,
		block_height ` + intType + ` NOT NULL,
		PRIMARY KEY (post_id, staker)
	);
	CREATE INDEX IF NOT EXISTS idx_post_stakes_staker ON post_stakes(staker);

	CREATE TABLE IF NOT EXISTS sync_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	// Add columns to existing tables (idempotent — ignores "already exists" errors).
	d.addColumnIfMissing("posts", "withdrawn", "INTEGER NOT NULL DEFAULT 0")
	d.addColumnIfMissing("posts", "total_burned", intType+" NOT NULL DEFAULT 0")
	d.addColumnIfMissing("boosts", "author_reward", intType+" NOT NULL DEFAULT 0")
	d.addColumnIfMissing("boosts", "staker_reward", intType+" NOT NULL DEFAULT 0")
	d.addColumnIfMissing("boosts", "burn_amount", intType+" NOT NULL DEFAULT 0")
	d.addColumnIfMissing("boosts", "staked_amount", intType+" NOT NULL DEFAULT 0")

	return nil
}

// addColumnIfMissing tries to add a column; silently ignores "already exists" errors.
func (d *DB) addColumnIfMissing(table, column, colType string) {
	q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType)
	d.db.Exec(q) // ignore error — column already exists
}

// autoIncPK returns the dialect-appropriate auto-increment primary key clause.
func (d *DB) autoIncPK() string {
	if d.driver == "pgx" {
		return "SERIAL PRIMARY KEY"
	}
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

// InsertPost inserts a new post.
func (d *DB) InsertPost(p *IndexedPost) error {
	q := fmt.Sprintf(
		`%s posts (post_id, author, text, channel, parent_post_id,
			created_at_height, created_at_time,
			total_staked, author_staked, third_party_staked, staker_count,
			unique_booster_count, last_boost_at_height, reply_count, withdrawn, total_burned)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)%s`,
		d.insertIgnore(), d.onConflictIgnore())
	_, err := d.exec(q,
		p.PostID, p.Author, p.Text, p.Channel, p.ParentPostID,
		p.CreatedAtHeight, p.CreatedAtTime,
		p.TotalStaked, p.AuthorStaked, p.ThirdPartyStaked, p.StakerCount,
		p.UniqueBoosterCount, p.LastBoostAtHeight, p.ReplyCount,
		boolToInt(p.Withdrawn), p.TotalBurned,
	)
	if err != nil {
		return err
	}
	// If this is a reply, increment the parent's reply count.
	if p.ParentPostID != "" {
		d.exec("UPDATE posts SET reply_count = reply_count + 1 WHERE post_id = ?", p.ParentPostID)
	}
	return nil
}

// InsertBoost inserts a boost record and updates the post's derived fields.
func (d *DB) InsertBoost(b *IndexedBoost, postAuthor string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert boost record.
	q := fmt.Sprintf(`%s boosts (tx_hash, post_id, booster, amount, author_reward, staker_reward, burn_amount, staked_amount, block_height, block_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)%s`, d.insertIgnore(), d.onConflictIgnore())
	_, err = tx.Exec(d.rewrite(q), b.TxHash, b.PostID, b.Booster, b.Amount, b.AuthorReward, b.StakerReward, b.BurnAmount, b.StakedAmount, b.BlockHeight, b.BlockTime)
	if err != nil {
		return err
	}

	// Update post totals (only the staked portion, not the full boost amount).
	isAuthor := b.Booster == postAuthor
	if isAuthor {
		_, err = tx.Exec(d.rewrite(
			`UPDATE posts SET
				total_staked = total_staked + ?,
				author_staked = author_staked + ?,
				total_burned = total_burned + ?,
				staker_count = staker_count + 1,
				last_boost_at_height = ?
			WHERE post_id = ?`),
			b.StakedAmount, b.StakedAmount, b.BurnAmount, b.BlockHeight, b.PostID,
		)
	} else {
		_, err = tx.Exec(d.rewrite(
			`UPDATE posts SET
				total_staked = total_staked + ?,
				third_party_staked = third_party_staked + ?,
				total_burned = total_burned + ?,
				staker_count = staker_count + 1,
				last_boost_at_height = ?
			WHERE post_id = ?`),
			b.StakedAmount, b.StakedAmount, b.BurnAmount, b.BlockHeight, b.PostID,
		)
	}
	if err != nil {
		return err
	}

	// Recompute unique booster count.
	_, err = tx.Exec(d.rewrite(
		`UPDATE posts SET unique_booster_count = (
			SELECT COUNT(DISTINCT booster) FROM boosts WHERE post_id = ?
		) WHERE post_id = ?`),
		b.PostID, b.PostID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// InsertTransfer inserts a transfer record.
func (d *DB) InsertTransfer(t *IndexedTransfer) error {
	q := fmt.Sprintf(`%s transfers (tx_hash, sender, recipient, amount, block_height, block_time)
		VALUES (?, ?, ?, ?, ?, ?)%s`, d.insertIgnore(), d.onConflictIgnore())
	_, err := d.exec(q,
		t.TxHash, t.Sender, t.Recipient, t.Amount, t.BlockHeight, t.BlockTime,
	)
	return err
}

// GetPost retrieves a single post by ID.
func (d *DB) GetPost(postID string) (*IndexedPost, error) {
	p := &IndexedPost{}
	var withdrawn int
	err := d.queryRow(
		`SELECT post_id, author, text, channel, parent_post_id,
			created_at_height, created_at_time,
			total_staked, author_staked, third_party_staked,
			staker_count, unique_booster_count, last_boost_at_height, reply_count,
			withdrawn, total_burned
		FROM posts WHERE post_id = ?`, postID,
	).Scan(&p.PostID, &p.Author, &p.Text, &p.Channel, &p.ParentPostID,
		&p.CreatedAtHeight, &p.CreatedAtTime,
		&p.TotalStaked, &p.AuthorStaked, &p.ThirdPartyStaked,
		&p.StakerCount, &p.UniqueBoosterCount, &p.LastBoostAtHeight, &p.ReplyCount,
		&withdrawn, &p.TotalBurned)
	p.Withdrawn = withdrawn != 0
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetChannels returns all channels with post counts.
func (d *DB) GetChannels() ([]ChannelInfo, error) {
	rows, err := d.query(
		`SELECT channel, COUNT(*) as cnt FROM posts WHERE parent_post_id = '' GROUP BY channel ORDER BY cnt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []ChannelInfo
	for rows.Next() {
		var c ChannelInfo
		rows.Scan(&c.Channel, &c.PostCount)
		channels = append(channels, c)
	}
	return channels, nil
}

// GetReplies returns replies to a post, sorted by committed value.
func (d *DB) GetReplies(postID string, page, pageSize int) ([]IndexedPost, int, error) {
	var total int
	d.queryRow("SELECT COUNT(*) FROM posts WHERE parent_post_id = ?", postID).Scan(&total)
	offset := (page - 1) * pageSize
	rows, err := d.query(
		`SELECT post_id, author, text, channel, parent_post_id,
			created_at_height, created_at_time,
			total_staked, author_staked, third_party_staked,
			staker_count, unique_booster_count, last_boost_at_height, reply_count,
			withdrawn, total_burned
		FROM posts WHERE parent_post_id = ? ORDER BY total_staked DESC LIMIT ? OFFSET ?`,
		postID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var replies []IndexedPost
	for rows.Next() {
		var p IndexedPost
		var withdrawn int
		rows.Scan(&p.PostID, &p.Author, &p.Text, &p.Channel, &p.ParentPostID,
			&p.CreatedAtHeight, &p.CreatedAtTime,
			&p.TotalStaked, &p.AuthorStaked, &p.ThirdPartyStaked,
			&p.StakerCount, &p.UniqueBoosterCount, &p.LastBoostAtHeight, &p.ReplyCount,
			&withdrawn, &p.TotalBurned)
		p.Withdrawn = withdrawn != 0
		replies = append(replies, p)
	}
	return replies, total, nil
}

// GetBoostsForPost retrieves boost history for a post, paginated.
func (d *DB) GetBoostsForPost(postID string, page, pageSize int) ([]IndexedBoost, int, error) {
	var total int
	d.queryRow("SELECT COUNT(*) FROM boosts WHERE post_id = ?", postID).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := d.query(
		`SELECT tx_hash, post_id, booster, amount, author_reward, staker_reward, burn_amount, staked_amount, block_height, block_time
		FROM boosts WHERE post_id = ? ORDER BY block_height ASC LIMIT ? OFFSET ?`,
		postID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var boosts []IndexedBoost
	for rows.Next() {
		var b IndexedBoost
		rows.Scan(&b.TxHash, &b.PostID, &b.Booster, &b.Amount, &b.AuthorReward, &b.StakerReward, &b.BurnAmount, &b.StakedAmount, &b.BlockHeight, &b.BlockTime)
		boosts = append(boosts, b)
	}
	return boosts, total, nil
}

// GetLastIndexedHeight returns the last indexed block height.
func (d *DB) GetLastIndexedHeight() (uint64, error) {
	var val string
	err := d.queryRow("SELECT value FROM sync_state WHERE key = 'last_indexed_height'").Scan(&val)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var h uint64
	fmt.Sscanf(val, "%d", &h)
	return h, nil
}

// SetLastIndexedHeight updates the sync cursor.
func (d *DB) SetLastIndexedHeight(h uint64) error {
	var q string
	if d.driver == "pgx" {
		q = `INSERT INTO sync_state (key, value) VALUES ('last_indexed_height', ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`
	} else {
		q = `INSERT OR REPLACE INTO sync_state (key, value) VALUES ('last_indexed_height', ?)`
	}
	_, err := d.exec(q, fmt.Sprintf("%d", h))
	return err
}

// GetChainStats returns aggregate statistics.
func (d *DB) GetChainStats(lastHeight uint64, totalBurned, totalIssued uint64) *ChainStats {
	stats := &ChainStats{
		LatestHeight:  lastHeight,
		TotalBurned:   totalBurned,
		TotalIssued:   totalIssued,
	}
	d.queryRow("SELECT COUNT(*) FROM posts").Scan(&stats.TotalPosts)
	d.queryRow("SELECT COUNT(*) FROM boosts").Scan(&stats.TotalBoosts)
	d.queryRow("SELECT COUNT(*) FROM transfers").Scan(&stats.TotalTransfers)
	if totalIssued >= totalBurned {
		stats.CirculatingSupply = totalIssued - totalBurned
	}
	return stats
}

// GetAuthorProfile returns aggregate stats for an author.
func (d *DB) GetAuthorProfile(address string) (*AuthorProfile, error) {
	ap := &AuthorProfile{Address: address}

	d.queryRow("SELECT COUNT(*) FROM posts WHERE author = ?", address).Scan(&ap.PostCount)
	if ap.PostCount == 0 {
		return nil, nil
	}

	d.queryRow(
		"SELECT COALESCE(SUM(total_staked), 0) FROM posts WHERE author = ?", address,
	).Scan(&ap.TotalStaked)

	d.queryRow(
		"SELECT COALESCE(SUM(third_party_staked), 0) FROM posts WHERE author = ?", address,
	).Scan(&ap.TotalReceived)

	d.queryRow(
		`SELECT COUNT(DISTINCT booster) FROM boosts
		WHERE post_id IN (SELECT post_id FROM posts WHERE author = ?) AND booster != ?`,
		address, address,
	).Scan(&ap.UniqueBoosterCount)

	return ap, nil
}

// GetLeaderboard returns authors sorted by total boosts received.
func (d *DB) GetLeaderboard(page, pageSize int) ([]LeaderboardEntry, int, error) {
	var total int
	d.queryRow("SELECT COUNT(DISTINCT author) FROM posts").Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := d.query(
		`SELECT author,
			COALESCE(SUM(third_party_staked), 0) as total_received,
			COUNT(*) as post_count,
			COALESCE(SUM(staker_count), 0) as staker_count
		FROM posts GROUP BY author
		ORDER BY total_received DESC
		LIMIT ? OFFSET ?`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	for rows.Next() {
		var e LeaderboardEntry
		rows.Scan(&e.Address, &e.TotalReceived, &e.PostCount, &e.StakerCount)
		entries = append(entries, e)
	}
	return entries, total, nil
}

// --- Reward Events ---

// InsertRewardEvent records a reward payment.
func (d *DB) InsertRewardEvent(postID, recipient, triggerTx, triggerAddr, rewardType string, amount, height uint64, blockTime int64) error {
	_, err := d.exec(
		`INSERT INTO reward_events (post_id, recipient, amount, block_height, block_time, trigger_tx, trigger_address, reward_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		postID, recipient, amount, height, blockTime, triggerTx, triggerAddr, rewardType,
	)
	return err
}

// GetRewardsByAddress returns paginated reward events for an address.
func (d *DB) GetRewardsByAddress(address string, sinceHeight uint64, page, pageSize int) ([]RewardEvent, int, uint64, error) {
	var total int
	var totalAmount uint64
	d.queryRow("SELECT COUNT(*), COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND block_height >= ?",
		address, sinceHeight).Scan(&total, &totalAmount)

	offset := (page - 1) * pageSize
	rows, err := d.query(
		`SELECT post_id, recipient, amount, block_height, block_time, trigger_tx, trigger_address, reward_type
		FROM reward_events WHERE recipient = ? AND block_height >= ?
		ORDER BY block_height DESC LIMIT ? OFFSET ?`,
		address, sinceHeight, pageSize, offset,
	)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	var events []RewardEvent
	for rows.Next() {
		var ev RewardEvent
		rows.Scan(&ev.PostID, &ev.Recipient, &ev.Amount, &ev.BlockHeight, &ev.BlockTime,
			&ev.TriggerTx, &ev.TriggerAddress, &ev.Type)
		events = append(events, ev)
	}
	return events, total, totalAmount, nil
}

// GetRewardSummary returns aggregate reward stats for an address.
func (d *DB) GetRewardSummary(address string, nowUnix int64) (last24h, last7d, allTime uint64, err error) {
	d.queryRow("SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ?", address).Scan(&allTime)
	d.queryRow("SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND block_time > ?",
		address, nowUnix-86400).Scan(&last24h)
	d.queryRow("SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND block_time > ?",
		address, nowUnix-604800).Scan(&last7d)
	return
}

// GetRewardsForPost returns total rewards earned by an address from a specific post.
func (d *DB) GetRewardsForPost(address, postID string) (uint64, error) {
	var total uint64
	err := d.queryRow("SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND post_id = ?",
		address, postID).Scan(&total)
	return total, err
}

// --- Post Stakes ---

// UpsertPostStake inserts or updates a stake position.
func (d *DB) UpsertPostStake(postID, staker string, amount, height uint64) error {
	var q string
	if d.driver == "pgx" {
		q = `INSERT INTO post_stakes (post_id, staker, amount, block_height) VALUES (?, ?, ?, ?)
			ON CONFLICT (post_id, staker) DO UPDATE SET amount = post_stakes.amount + EXCLUDED.amount, block_height = EXCLUDED.block_height`
	} else {
		q = `INSERT INTO post_stakes (post_id, staker, amount, block_height) VALUES (?, ?, ?, ?)
			ON CONFLICT (post_id, staker) DO UPDATE SET amount = post_stakes.amount + excluded.amount, block_height = excluded.block_height`
	}
	_, err := d.exec(q, postID, staker, amount, height)
	return err
}

// RemovePostStake removes a single staker's position on a post.
func (d *DB) RemovePostStake(postID, staker string) error {
	_, err := d.exec("DELETE FROM post_stakes WHERE post_id = ? AND staker = ?", postID, staker)
	return err
}

// RemoveAllPostStakes removes all stake positions for a post (author withdrawal).
func (d *DB) RemoveAllPostStakes(postID string) error {
	_, err := d.exec("DELETE FROM post_stakes WHERE post_id = ?", postID)
	return err
}

// GetPostStakePositions returns all stakers for a post.
func (d *DB) GetPostStakePositions(postID string) []PostStakeRecord {
	rows, err := d.query(
		"SELECT post_id, staker, amount, block_height FROM post_stakes WHERE post_id = ?", postID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var stakes []PostStakeRecord
	for rows.Next() {
		var s PostStakeRecord
		rows.Scan(&s.PostID, &s.Staker, &s.Amount, &s.Height)
		stakes = append(stakes, s)
	}
	return stakes
}

// GetStakesByAddress returns all post stakes for an address.
func (d *DB) GetStakesByAddress(address string) ([]PostStakeRecord, uint64) {
	rows, err := d.query(
		"SELECT post_id, staker, amount, block_height FROM post_stakes WHERE staker = ? ORDER BY block_height DESC", address)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	var stakes []PostStakeRecord
	var total uint64
	for rows.Next() {
		var s PostStakeRecord
		rows.Scan(&s.PostID, &s.Staker, &s.Amount, &s.Height)
		stakes = append(stakes, s)
		total += s.Amount
	}
	return stakes, total
}
