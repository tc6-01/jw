package localstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jw/internal/domain/urlnorm"
)

var ErrNoMatch = errors.New("no matched destination")

const (
	SourceManual = "manual"
	SourceAuto   = "auto"
	SourceLegacy = "legacy"

	DepthBucketShallow = "d0-1"
	DepthBucketMedium  = "d2-3"
	DepthBucketDeep    = "d4+"
)

type Entry struct {
	URL            string `json:"url"`
	Title          string `json:"title,omitempty"`
	Count          int    `json:"count"`
	LastSeen       int64  `json:"last_seen"`
	Source         string `json:"source,omitempty"`
	GroupKey       string `json:"group_key,omitempty"`
	DepthBucket    string `json:"depth_bucket,omitempty"`
	TopicKey       string `json:"topic_key,omitempty"`
	Representative bool   `json:"representative,omitempty"`
}

type Match struct {
	Entry Entry
	Score float64
}

type DB struct {
	Entries []Entry `json:"entries"`
}

type CurationMetadata struct {
	HostKey     string
	DepthBucket string
	TopicKey    string
	GroupKey    string
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
	db.ensureCompatibility()
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
	return db.AddManual(rawURL, title)
}

func (db *DB) AddManual(rawURL, title string) (Entry, error) {
	return db.addWithSource(rawURL, title, SourceManual, time.Now().Unix())
}

func (db *DB) AddAuto(rawURL, title string, eventUnix int64) (Entry, error) {
	return db.addWithSource(rawURL, title, SourceAuto, eventUnix)
}

func (db *DB) addWithSource(rawURL, title, source string, eventUnix int64) (Entry, error) {
	normalized, err := urlnorm.NormalizeAndRedact(rawURL)
	if err != nil {
		return Entry{}, err
	}

	title = strings.TrimSpace(title)
	if eventUnix <= 0 {
		eventUnix = time.Now().Unix()
	}
	oldGroupKey := ""

	for i := range db.Entries {
		if db.Entries[i].URL != normalized {
			continue
		}

		entry := &db.Entries[i]
		currentSource := normalizeSource(entry.Source)
		if source == SourceAuto && currentSource == SourceManual {
			entry.Count++
			if eventUnix > entry.LastSeen {
				entry.LastSeen = eventUnix
			}
			if title != "" {
				entry.Title = title
			}
			return *entry, nil
		}

		oldGroupKey = entry.GroupKey
		entry.Count++
		if eventUnix > entry.LastSeen {
			entry.LastSeen = eventUnix
		}
		if title != "" {
			entry.Title = title
		}

		switch source {
		case SourceManual:
			entry.Source = SourceManual
			clearAutoMetadata(entry)
		case SourceAuto:
			entry.Source = SourceAuto
			meta, err := DeriveCurationMetadata(normalized)
			if err != nil {
				return Entry{}, err
			}
			entry.GroupKey = meta.GroupKey
			entry.DepthBucket = meta.DepthBucket
			entry.TopicKey = meta.TopicKey
		}

		if oldGroupKey != "" && oldGroupKey != entry.GroupKey {
			db.reconcileAutoGroup(oldGroupKey)
		}
		if normalizeSource(entry.Source) == SourceAuto {
			db.reconcileAutoGroup(entry.GroupKey)
		}
		return *entry, nil
	}

	entry := Entry{
		URL:      normalized,
		Title:    title,
		Count:    1,
		LastSeen: eventUnix,
		Source:   normalizeSource(source),
	}
	if entry.Source == SourceAuto {
		meta, err := DeriveCurationMetadata(normalized)
		if err != nil {
			return Entry{}, err
		}
		entry.GroupKey = meta.GroupKey
		entry.DepthBucket = meta.DepthBucket
		entry.TopicKey = meta.TopicKey
	}

	db.Entries = append(db.Entries, entry)
	if entry.Source == SourceAuto {
		db.reconcileAutoGroup(entry.GroupKey)
	}
	return entry, nil
}

func (db *DB) Touch(rawURL string) bool {
	normalized, err := urlnorm.NormalizeAndRedact(rawURL)
	if err != nil {
		return false
	}
	return db.TouchNormalized(normalized)
}

func (db *DB) TouchNormalized(normalizedURL string) bool {
	now := time.Now().Unix()
	for i := range db.Entries {
		if db.Entries[i].URL != normalizedURL {
			continue
		}
		db.Entries[i].Count++
		db.Entries[i].LastSeen = now
		if normalizeSource(db.Entries[i].Source) == SourceAuto {
			db.reconcileAutoGroup(db.Entries[i].GroupKey)
		}
		return true
	}
	return false
}

func (db *DB) Remove(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}

	for i := range db.Entries {
		if db.Entries[i].URL == target || db.Entries[i].Title == target {
			removed := db.Entries[i]
			db.Entries = append(db.Entries[:i], db.Entries[i+1:]...)
			if normalizeSource(removed.Source) == SourceAuto {
				db.reconcileAutoGroup(removed.GroupKey)
			}
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
		e.Source = normalizeSource(e.Source)
		if e.Source == SourceAuto && !e.Representative {
			continue
		}
		score := score(keyword, e, now)
		if score <= 0 {
			continue
		}
		matches = append(matches, Match{Entry: e, Score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			if matches[i].Entry.LastSeen == matches[j].Entry.LastSeen {
				return entryDepth(matches[i].Entry.URL) > entryDepth(matches[j].Entry.URL)
			}
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
	matches := db.Query(keyword, 20)
	if len(matches) == 0 {
		return Match{}, ErrNoMatch
	}

	baseline := matches[0].Score * 0.90
	if baseline < 0 {
		baseline = matches[0].Score
	}

	var deepCandidates []Match
	for _, m := range matches {
		if m.Score < baseline {
			break
		}
		if isDeepPage(m.Entry.URL) {
			deepCandidates = append(deepCandidates, m)
		}
	}

	if len(deepCandidates) == 0 {
		return matches[0], nil
	}

	best := deepCandidates[0]
	for i := 1; i < len(deepCandidates); i++ {
		if deepCandidates[i].Entry.LastSeen > best.Entry.LastSeen {
			best = deepCandidates[i]
		}
	}
	return best, nil
}

func DeriveCurationMetadata(normalizedURL string) (CurationMetadata, error) {
	u, err := url.Parse(normalizedURL)
	if err != nil {
		return CurationMetadata{}, err
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		host = strings.ToLower(strings.TrimSpace(u.Host))
	}
	if host == "" {
		return CurationMetadata{}, fmt.Errorf("invalid host in url: %s", normalizedURL)
	}

	depth := countPathDepth(u.Path)
	bucket := depthToBucket(depth)
	topic := topicKeyFromPath(u.Path)
	group := fmt.Sprintf("%s|%s|%s", host, bucket, topic)

	return CurationMetadata{
		HostKey:     host,
		DepthBucket: bucket,
		TopicKey:    topic,
		GroupKey:    group,
	}, nil
}

func score(keyword string, e Entry, now time.Time) float64 {
	if keyword == "" {
		return math.Max(1, float64(e.Count)) * sourceWeight(e.Source)
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
	return base * decay * ctx * sourceWeight(e.Source)
}

func sourceWeight(source string) float64 {
	switch normalizeSource(source) {
	case SourceManual:
		return 1.35
	case SourceAuto:
		return 0.95
	default:
		return 1.0
	}
}

func normalizeSource(source string) string {
	s := strings.ToLower(strings.TrimSpace(source))
	switch s {
	case SourceManual, SourceAuto, SourceLegacy:
		return s
	default:
		return SourceLegacy
	}
}

func clearAutoMetadata(e *Entry) {
	e.GroupKey = ""
	e.DepthBucket = ""
	e.TopicKey = ""
	e.Representative = false
}

func (db *DB) ensureCompatibility() {
	now := time.Now().Unix()
	for i := range db.Entries {
		e := &db.Entries[i]
		if e.Count <= 0 {
			e.Count = 1
		}
		if e.LastSeen <= 0 {
			e.LastSeen = now
		}
		e.Title = strings.TrimSpace(e.Title)
		e.Source = normalizeSource(e.Source)

		if e.Source == SourceAuto {
			if e.GroupKey == "" || e.DepthBucket == "" || e.TopicKey == "" {
				meta, err := DeriveCurationMetadata(e.URL)
				if err == nil {
					e.GroupKey = meta.GroupKey
					e.DepthBucket = meta.DepthBucket
					e.TopicKey = meta.TopicKey
				}
			}
		} else {
			e.Representative = false
		}
	}
	db.reconcileAllAutoRepresentatives()
}

func (db *DB) reconcileAllAutoRepresentatives() {
	groups := map[string]struct{}{}
	for i := range db.Entries {
		e := &db.Entries[i]
		if normalizeSource(e.Source) != SourceAuto {
			continue
		}
		if e.GroupKey == "" {
			meta, err := DeriveCurationMetadata(e.URL)
			if err != nil {
				continue
			}
			e.GroupKey = meta.GroupKey
			e.DepthBucket = meta.DepthBucket
			e.TopicKey = meta.TopicKey
		}
		groups[e.GroupKey] = struct{}{}
	}
	for g := range groups {
		db.reconcileAutoGroup(g)
	}
}

func (db *DB) reconcileAutoGroup(groupKey string) {
	if strings.TrimSpace(groupKey) == "" {
		return
	}

	bestIdx := -1
	for i := range db.Entries {
		e := &db.Entries[i]
		if normalizeSource(e.Source) != SourceAuto || e.GroupKey != groupKey {
			continue
		}
		if bestIdx == -1 || isBetterRepresentative(*e, db.Entries[bestIdx]) {
			bestIdx = i
		}
	}

	for i := range db.Entries {
		e := &db.Entries[i]
		if normalizeSource(e.Source) == SourceAuto && e.GroupKey == groupKey {
			e.Representative = (i == bestIdx)
		}
	}
}

func isBetterRepresentative(a, b Entry) bool {
	if a.LastSeen != b.LastSeen {
		return a.LastSeen > b.LastSeen
	}
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	return a.URL < b.URL
}

func countPathDepth(path string) int {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return 0
	}
	parts := strings.Split(trimmed, "/")
	depth := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			depth++
		}
	}
	return depth
}

func depthToBucket(depth int) string {
	switch {
	case depth <= 1:
		return DepthBucketShallow
	case depth <= 3:
		return DepthBucketMedium
	default:
		return DepthBucketDeep
	}
}

func topicKeyFromPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "_root"
	}
	parts := strings.Split(trimmed, "/")
	first := strings.ToLower(strings.TrimSpace(parts[0]))
	if first == "" {
		return "_root"
	}
	if len(first) > 48 {
		first = first[:48]
	}
	return first
}

func entryDepth(rawURL string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	return countPathDepth(u.Path)
}

func isDeepPage(rawURL string) bool {
	return entryDepth(rawURL) >= 1
}
