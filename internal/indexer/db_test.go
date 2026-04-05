package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertAndGetPost(t *testing.T) {
	db := tempDB(t)
	p := &IndexedPost{
		PostID: "abc123", Author: "drana1alice", Text: "hello world",
		CreatedAtHeight: 5, CreatedAtTime: 1700000000,
		TotalStaked: 1000000, AuthorStaked: 1000000,
	}
	if err := db.InsertPost(p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	got, err := db.GetPost("abc123")
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got == nil {
		t.Fatal("post not found")
	}
	if got.Text != "hello world" || got.TotalStaked != 1000000 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestInsertBoostThirdParty(t *testing.T) {
	db := tempDB(t)
	db.InsertPost(&IndexedPost{
		PostID: "post1", Author: "drana1alice", Text: "test",
		CreatedAtHeight: 1, CreatedAtTime: 1700000000,
		TotalStaked: 500, AuthorStaked: 500,
	})

	// 300 total: 9 burn (3%), 6 author (2%), 3 staker (1%), 282 staked (94%)
	boost := &IndexedBoost{
		PostID: "post1", Booster: "drana1bob", Amount: 300,
		AuthorReward: 6, StakerReward: 3, BurnAmount: 9, StakedAmount: 282,
		BlockHeight: 2, BlockTime: 1700000120, TxHash: "tx1",
	}
	if err := db.InsertBoost(boost, "drana1alice"); err != nil {
		t.Fatalf("InsertBoost: %v", err)
	}

	got, _ := db.GetPost("post1")
	if got.TotalStaked != 782 { // 500 + 282
		t.Fatalf("totalStaked: %d, want 782", got.TotalStaked)
	}
	if got.AuthorStaked != 500 {
		t.Fatalf("authorStaked: %d, want 500", got.AuthorStaked)
	}
	if got.ThirdPartyStaked != 282 {
		t.Fatalf("thirdPartyStaked: %d, want 282", got.ThirdPartyStaked)
	}
	if got.TotalBurned != 9 {
		t.Fatalf("totalBurned: %d, want 9", got.TotalBurned)
	}
	if got.StakerCount != 1 {
		t.Fatalf("boostCount: %d, want 1", got.StakerCount)
	}
	if got.UniqueBoosterCount != 1 {
		t.Fatalf("uniqueBoosterCount: %d, want 1", got.UniqueBoosterCount)
	}
	if got.LastBoostAtHeight != 2 {
		t.Fatalf("lastBoostAtHeight: %d, want 2", got.LastBoostAtHeight)
	}
}

func TestInsertBoostSelf(t *testing.T) {
	db := tempDB(t)
	db.InsertPost(&IndexedPost{
		PostID: "post1", Author: "drana1alice", Text: "test",
		CreatedAtHeight: 1, CreatedAtTime: 1700000000,
		TotalStaked: 500, AuthorStaked: 500,
	})

	// 200 total: 6 burn, 4 author, 2 staker, 188 staked
	boost := &IndexedBoost{
		PostID: "post1", Booster: "drana1alice", Amount: 200,
		AuthorReward: 4, StakerReward: 2, BurnAmount: 6, StakedAmount: 188,
		BlockHeight: 3, BlockTime: 1700000240, TxHash: "tx2",
	}
	if err := db.InsertBoost(boost, "drana1alice"); err != nil {
		t.Fatalf("InsertBoost: %v", err)
	}

	got, _ := db.GetPost("post1")
	if got.AuthorStaked != 688 { // 500 + 188
		t.Fatalf("authorStaked: %d, want 688", got.AuthorStaked)
	}
	if got.ThirdPartyStaked != 0 {
		t.Fatalf("thirdPartyStaked: %d, want 0", got.ThirdPartyStaked)
	}
}

func TestSyncStateRoundTrip(t *testing.T) {
	db := tempDB(t)

	h, _ := db.GetLastIndexedHeight()
	if h != 0 {
		t.Fatalf("initial height: %d", h)
	}

	db.SetLastIndexedHeight(42)
	h, _ = db.GetLastIndexedHeight()
	if h != 42 {
		t.Fatalf("height after set: %d", h)
	}
}

func TestGetPostNonexistent(t *testing.T) {
	db := tempDB(t)
	got, err := db.GetPost("doesnotexist")
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got != nil {
		t.Fatal("should be nil for nonexistent post")
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := tempDB(t)
	// Second migrate should not error.
	if err := db.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestLeaderboard(t *testing.T) {
	db := tempDB(t)
	db.InsertPost(&IndexedPost{
		PostID: "p1", Author: "alice", Text: "a", CreatedAtHeight: 1, CreatedAtTime: 100,
		TotalStaked: 500, AuthorStaked: 200, ThirdPartyStaked: 300, StakerCount: 2,
	})
	db.InsertPost(&IndexedPost{
		PostID: "p2", Author: "bob", Text: "b", CreatedAtHeight: 2, CreatedAtTime: 200,
		TotalStaked: 100, AuthorStaked: 100, ThirdPartyStaked: 0,
	})

	entries, total, err := db.GetLeaderboard(1, 10)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if total != 2 {
		t.Fatalf("total: %d", total)
	}
	if entries[0].Address != "alice" {
		t.Fatalf("first should be alice, got %s", entries[0].Address)
	}
	_ = os.TempDir() // satisfy import
}
