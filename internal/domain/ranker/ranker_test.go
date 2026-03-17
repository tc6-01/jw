package ranker

import "testing"

func TestBaseScore_RecentHigherThanOld(t *testing.T) {
	p := DefaultParams()
	recent := BaseScore([]Event{{DaysAgo: 1, AliasWeight: 1}}, p)
	old := BaseScore([]Event{{DaysAgo: 30, AliasWeight: 1}}, p)
	if recent <= old {
		t.Fatalf("expected recent > old, got recent=%f old=%f", recent, old)
	}
}

func TestContextWeight_IncreasesWithKeyword(t *testing.T) {
	p := DefaultParams()
	w1 := ContextWeight(Context{KeywordScore: 0.1, AliasBoost: 1}, p)
	w2 := ContextWeight(Context{KeywordScore: 0.9, AliasBoost: 1}, p)
	if w2 <= w1 {
		t.Fatalf("expected w2 > w1, got w1=%f w2=%f", w1, w2)
	}
}

func TestContextWeight_RespectsMaxCap(t *testing.T) {
	p := DefaultParams()
	ctx := Context{DirMatches: 1000, KeywordScore: 1, TimeBucketMatches: 1000, AliasBoost: 2}
	w := ContextWeight(ctx, p)
	if w > p.ContextMax {
		t.Fatalf("expected capped <= %f, got %f", p.ContextMax, w)
	}
}
