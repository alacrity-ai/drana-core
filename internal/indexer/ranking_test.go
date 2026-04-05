package indexer

import (
	"testing"
	"time"
)

func seedPostsForRanking(t *testing.T, db *DB) {
	t.Helper()
	now := time.Now().Unix()

	posts := []IndexedPost{
		{PostID: "old_big", Author: "alice", Text: "old big", CreatedAtHeight: 1, CreatedAtTime: now - 7200, TotalStaked: 10000000, AuthorStaked: 10000000},
		{PostID: "new_small", Author: "bob", Text: "new small", CreatedAtHeight: 10, CreatedAtTime: now - 60, TotalStaked: 100000, AuthorStaked: 100000},
		{PostID: "new_big", Author: "alice", Text: "new big", CreatedAtHeight: 9, CreatedAtTime: now - 120, TotalStaked: 5000000, AuthorStaked: 5000000},
		{PostID: "controversial", Author: "carol", Text: "hot take", CreatedAtHeight: 5, CreatedAtTime: now - 3600, TotalStaked: 500000, AuthorStaked: 100000, ThirdPartyStaked: 400000, StakerCount: 20, UniqueBoosterCount: 15},
	}
	for _, p := range posts {
		if err := db.InsertPost(&p); err != nil {
			t.Fatalf("InsertPost %s: %v", p.PostID, err)
		}
	}
}

func TestRankByTop(t *testing.T) {
	db := tempDB(t)
	seedPostsForRanking(t, db)

	results, total, err := db.RankedPosts(RankByTopAllTime, 1, 10, time.Now().Unix(), "")
	if err != nil {
		t.Fatalf("RankedPosts: %v", err)
	}
	if total != 4 {
		t.Fatalf("total: %d", total)
	}
	// old_big (10M) should be first.
	if results[0].PostID != "old_big" {
		t.Fatalf("top: first should be old_big, got %s", results[0].PostID)
	}
	// new_big (5M) should be second.
	if results[1].PostID != "new_big" {
		t.Fatalf("top: second should be new_big, got %s", results[1].PostID)
	}
}

func TestRankByNew(t *testing.T) {
	db := tempDB(t)
	seedPostsForRanking(t, db)

	results, _, _ := db.RankedPosts(RankByNew, 1, 10, time.Now().Unix(), "")
	// new_small (height 10) should be first.
	if results[0].PostID != "new_small" {
		t.Fatalf("new: first should be new_small, got %s", results[0].PostID)
	}
}

func TestRankByTrending(t *testing.T) {
	db := tempDB(t)
	seedPostsForRanking(t, db)

	results, _, _ := db.RankedPosts(RankByTrending, 1, 10, time.Now().Unix(), "")
	// new_big (5M, 2 min old) should beat old_big (10M, 2 hours old) on trending.
	if results[0].PostID != "new_big" {
		t.Fatalf("trending: first should be new_big, got %s (score=%.4f)", results[0].PostID, results[0].Score)
	}
}

func TestRankByControversial(t *testing.T) {
	db := tempDB(t)
	seedPostsForRanking(t, db)

	results, _, _ := db.RankedPosts(RankByControversial, 1, 10, time.Now().Unix(), "")
	// controversial (20 boosts * log(500001)) should rank high.
	if results[0].PostID != "controversial" {
		t.Fatalf("controversial: first should be controversial, got %s", results[0].PostID)
	}
}

func TestRankingPagination(t *testing.T) {
	db := tempDB(t)
	seedPostsForRanking(t, db)

	page1, total, _ := db.RankedPosts(RankByTopAllTime, 1, 2, time.Now().Unix(), "")
	if total != 4 {
		t.Fatalf("total: %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len: %d", len(page1))
	}

	page2, _, _ := db.RankedPosts(RankByTopAllTime, 2, 2, time.Now().Unix(), "")
	if len(page2) != 2 {
		t.Fatalf("page2 len: %d", len(page2))
	}

	// No overlap.
	if page1[0].PostID == page2[0].PostID {
		t.Fatal("pages overlap")
	}
}

func TestRankByAuthor(t *testing.T) {
	db := tempDB(t)
	seedPostsForRanking(t, db)

	results, total, _ := db.RankedPostsByAuthor("alice", RankByTopAllTime, 1, 10, time.Now().Unix())
	if total != 2 {
		t.Fatalf("alice total: %d", total)
	}
	if len(results) != 2 {
		t.Fatalf("alice results: %d", len(results))
	}
	for _, r := range results {
		if r.Author != "alice" {
			t.Fatalf("non-alice post in results: %s", r.Author)
		}
	}
}
