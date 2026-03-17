package localstore

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jw/internal/domain/urlnorm"
)

var ErrNoMatch = errors.New("no matched destination")

type Entry struct {
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Count    int    `json:"count"`
	LastSeen int64  `json:"last_seen"`
}

type Match struct {
	Entry Entry
	Score float64
}

type DB struct {
	Entries []Entry `json:"entries"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".jw", "store.json"), nil
}

func Load(path string) (*DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &DB{Entries: []Entry{}}, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return &DB{Entries: []Entry{}}, nil
	}

	var db DB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}
	if db.Entries == nil {
		db.Entries = []Entry{}
	}
	return &db, nil
}

func (db *DB) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (db *DB) Add(rawURL, title string) (Entry, error) {
	normalized, err := urlnorm.NormalizeAndRedact(rawURL)
	if err != nil {
		return Entry{}, err
	}

	now := time.Now().Unix()
	for i := range db.Entries {
		if db.Entries[i].URL == normalized {
			db.Entries[i].Count++
			db.Entries[i].LastSeen = now
			if strings.TrimSpace(title) != "" {
				db.Entries[i].Title = strings.TrimSpace(title)
			}
			return db.Entries[i], nil
		}
	}

	entry := Entry{
		URL:      normalized,
		Title:    strings.TrimSpace(title),
		Count:    1,
		LastSeen: now,
	}
	db.Entries = append(db.Entries, entry)
	return entry, nil
}

func (db *DB) Remove(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}

	for i := range db.Entries {
		if db.Entries[i].URL == target || db.Entries[i].Title == target {
			db.Entries = append(db.Entries[:i], db.Entries[i+1:]...)
			return true
		}
	}
	return false
}

func (db *DB) Query(keyword string, limit int) []Match {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if limit <= 0 {
		limit = 5
	}

	now := time.Now()
	matches := make([]Match, 0, len(db.Entries))
	for _, e := range db.Entries {
		score := score(keyword, e, now)
		if score <= 0 {
			continue
		}
		matches = append(matches, Match{Entry: e, Score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Entry.LastSeen > matches[j].Entry.LastSeen
		}
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > limit {
		return matches[:limit]
	}
	return matches
}

func (db *DB) Best(keyword string) (Match, error) {
	matches := db.Query(keyword, 1)
	if len(matches) == 0 {
		return Match{}, ErrNoMatch
	}
	return matches[0], nil
}

func score(keyword string, e Entry, now time.Time) float64 {
	if keyword == "" {
		return float64(e.Count)
	}

	urlLower := strings.ToLower(e.URL)
	titleLower := strings.ToLower(e.Title)

	kwScore := 0.0
	switch {
	case strings.Contains(titleLower, keyword):
		kwScore = 1.0
	case strings.Contains(urlLower, keyword):
		kwScore = 0.8
	default:
		return 0
	}

	daysAgo := now.Sub(time.Unix(e.LastSeen, 0)).Hours() / 24
	if daysAgo < 0 {
		daysAgo = 0
	}
	lambda := math.Ln2 / 14.0
	decay := math.Exp(-lambda * daysAgo)

	base := math.Max(1, float64(e.Count))
	ctx := 1 + 0.9*kwScore
	return base * decay * ctx
}
