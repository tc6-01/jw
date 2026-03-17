package ranker

import "math"

type Event struct {
	DaysAgo     float64
	AliasWeight float64
}

type Context struct {
	DirMatches        int
	KeywordScore      float64
	TimeBucketMatches int
	AliasBoost        float64
}

type Params struct {
	HalfLifeDays float64
	BetaDir      float64
	BetaKeyword  float64
	ContextMax   float64
	ColdStartEps float64
}

func DefaultParams() Params {
	return Params{
		HalfLifeDays: 14,
		BetaDir:      0.35,
		BetaKeyword:  0.9,
		ContextMax:   2.5,
		ColdStartEps: 0.05,
	}
}

func Score(events []Event, ctx Context, p Params) float64 {
	base := BaseScore(events, p)
	return (base + p.ColdStartEps) * ContextWeight(ctx, p)
}

func BaseScore(events []Event, p Params) float64 {
	if p.HalfLifeDays <= 0 {
		p.HalfLifeDays = 14
	}
	lambda := math.Ln2 / p.HalfLifeDays

	total := 0.0
	for _, e := range events {
		w := e.AliasWeight
		if w <= 0 {
			w = 1.0
		}
		days := math.Max(0, e.DaysAgo)
		total += w * math.Exp(-lambda*days)
	}
	return total
}

func ContextWeight(ctx Context, p Params) float64 {
	wDir := 1 + math.Min(0.6, 0.15*math.Log1p(float64(max(0, ctx.DirMatches))))

	kw := clamp(ctx.KeywordScore, 0, 1)
	wKW := 1 + p.BetaKeyword*kw

	wTOD := 1 + math.Min(0.25, 0.08*math.Log1p(float64(max(0, ctx.TimeBucketMatches))))

	wAlias := ctx.AliasBoost
	if wAlias <= 0 {
		wAlias = 1.0
	}

	w := wDir * wKW * wTOD * wAlias
	if p.ContextMax > 0 {
		w = math.Min(w, p.ContextMax)
	}
	return w
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
