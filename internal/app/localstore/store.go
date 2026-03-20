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

var (
	ErrNoMatch       = errors.New("no matched destination")
	ErrIgnoredByRule = errors.New("url ignored by rule")
)

const (
	SourceManual = "manual"
	SourceAuto   = "auto"
	SourceLegacy = "legacy"

	RuleAlias         = "alias"
	RuleIgnore        = "ignore"
	RuleCollapse      = "collapse"
	RuleDefault       = "default"
	RulePreserveQuery = "preserve_query"

	schemaVersion = 2
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
	Entry   Entry
	Score   float64
	Reason  string
	NodeKey string
}

type Target struct {
	Key      string `json:"key"`
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Count    int    `json:"count"`
	LastSeen int64  `json:"last_seen"`
	Source   string `json:"source,omitempty"`
	HostKey  string `json:"host_key,omitempty"`
	NodePath string `json:"node_path,omitempty"`
	NodeKey  string `json:"node_key,omitempty"`
}

type Node struct {
	Key              string `json:"key"`
	HostKey          string `json:"host_key"`
	Path             string `json:"path,omitempty"`
	ParentKey        string `json:"parent_key,omitempty"`
	Count            int    `json:"count"`
	LastSeen         int64  `json:"last_seen"`
	DefaultTargetKey string `json:"default_target_key,omitempty"`
	ManualTargetKey  string `json:"manual_target_key,omitempty"`
	ExactTargetKey   string `json:"exact_target_key,omitempty"`
}

type Rule struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern,omitempty"`
	Value   string `json:"value,omitempty"`
	Host    string `json:"host,omitempty"`
}

type Metadata struct {
	SchemaVersion       int   `json:"schema_version,omitempty"`
	MigratedFromEntries bool  `json:"migrated_from_entries,omitempty"`
	LegacyEntryCount    int   `json:"legacy_entry_count,omitempty"`
	LastMigratedAt      int64 `json:"last_migrated_at,omitempty"`
}

type NodeSummary struct {
	Key          string
	HostKey      string
	Path         string
	Depth        int
	Count        int
	LastSeen     int64
	DefaultURL   string
	DefaultTitle string
}

type Store interface {
	Save(path string) error
	Add(rawURL, title string) (Entry, error)
	AddManual(rawURL, title string) (Entry, error)
	AddAuto(rawURL, title string, eventUnix int64) (Entry, error)
	Touch(rawURL string) bool
	TouchNormalized(normalizedURL string) bool
	Remove(target string) bool
	Query(keyword string, limit int) []Match
	Best(keyword string) (Match, error)
	ListNodes() []NodeSummary
}

type DB struct {
	Entries  []Entry  `json:"entries,omitempty"`
	Targets  []Target `json:"targets,omitempty"`
	Nodes    []Node   `json:"nodes,omitempty"`
	Rules    []Rule   `json:"rules,omitempty"`
	Metadata Metadata `json:"metadata,omitempty"`
}

type canonicalTarget struct {
	TargetURL string
	HostKey   string
	NodePath  string
	NodeKey   string
	NodeChain []nodeRef
}

type nodeRef struct {
	Key       string
	HostKey   string
	Path      string
	ParentKey string
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
			db := &DB{}
			db.ensureCompatibility()
			return db, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		db := &DB{}
		db.ensureCompatibility()
		return db, nil
	}

	var db DB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}
	db.ensureCompatibility()
	return &db, nil
}

func (db *DB) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	db.ensureCompatibility()

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
	canonical, err := db.deriveCanonicalTarget(rawURL)
	if err != nil {
		return Entry{}, err
	}
	if eventUnix <= 0 {
		eventUnix = time.Now().Unix()
	}

	source = normalizeSource(source)
	title = strings.TrimSpace(title)

	existing := db.findTargetByURL(canonical.TargetURL)
	if existing != nil {
		existing.Count++
		if eventUnix > existing.LastSeen {
			existing.LastSeen = eventUnix
		}
		if title != "" {
			existing.Title = title
		}
		if source == SourceManual {
			existing.Source = SourceManual
		} else if existing.Source != SourceManual {
			existing.Source = source
		}
		existing.HostKey = canonical.HostKey
		existing.NodePath = canonical.NodePath
		existing.NodeKey = canonical.NodeKey
		db.rebuildDerivedState()
		return targetToEntry(*existing), nil
	}

	target := Target{
		Key:      canonical.TargetURL,
		URL:      canonical.TargetURL,
		Title:    title,
		Count:    1,
		LastSeen: eventUnix,
		Source:   source,
		HostKey:  canonical.HostKey,
		NodePath: canonical.NodePath,
		NodeKey:  canonical.NodeKey,
	}
	db.Targets = append(db.Targets, target)
	db.rebuildDerivedState()
	return targetToEntry(target), nil
}

func (db *DB) Touch(rawURL string) bool {
	canonical, err := db.deriveCanonicalTarget(rawURL)
	if err != nil {
		return false
	}
	return db.touchTargetURL(canonical.TargetURL)
}

func (db *DB) TouchNormalized(normalizedURL string) bool {
	canonical, err := db.deriveCanonicalTargetFromNormalized(normalizedURL)
	if err != nil {
		return false
	}
	return db.touchTargetURL(canonical.TargetURL)
}

func (db *DB) touchTargetURL(targetURL string) bool {
	target := db.findTargetByURL(targetURL)
	if target == nil {
		return false
	}
	target.Count++
	target.LastSeen = time.Now().Unix()
	db.rebuildDerivedState()
	return true
}

func (db *DB) Remove(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}

	for i := range db.Targets {
		if db.Targets[i].URL == target || db.Targets[i].Title == target {
			db.Targets = append(db.Targets[:i], db.Targets[i+1:]...)
			db.rebuildDerivedState()
			return true
		}
	}
	return false
}

func (db *DB) Query(keyword string, limit int) []Match {
	keyword = strings.TrimSpace(keyword)
	if limit <= 0 {
		limit = 5
	}

	if keyword == "" {
		return nil
	}

	if direct, ok := db.resolveAddressMatch(keyword); ok {
		return []Match{direct}
	}

	now := time.Now()
	lowerKeyword := strings.ToLower(keyword)
	matches := make([]Match, 0, len(db.Nodes))
	seenTargets := map[string]struct{}{}
	for _, node := range db.Nodes {
		target := db.resolveLandingTarget(node.Key, map[string]bool{})
		if target == nil {
			continue
		}
		reason, kwScore := fuzzyMatchReason(lowerKeyword, node, *target)
		if kwScore <= 0 {
			continue
		}
		if _, exists := seenTargets[target.URL]; exists {
			continue
		}
		seenTargets[target.URL] = struct{}{}
		matches = append(matches, Match{
			Entry:   targetToEntry(*target),
			Score:   scoreNode(kwScore, node, *target, now),
			Reason:  reason,
			NodeKey: node.Key,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			if matches[i].Entry.LastSeen == matches[j].Entry.LastSeen {
				return pathDepth(matches[i].NodeKey) > pathDepth(matches[j].NodeKey)
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
	return matches[0], nil
}

func (db *DB) ListNodes() []NodeSummary {
	summaries := make([]NodeSummary, 0, len(db.Nodes))
	for _, node := range db.Nodes {
		target := db.resolveLandingTarget(node.Key, map[string]bool{})
		if target == nil {
			continue
		}
		summaries = append(summaries, NodeSummary{
			Key:          node.Key,
			HostKey:      node.HostKey,
			Path:         node.Path,
			Depth:        pathDepth(node.Key),
			Count:        node.Count,
			LastSeen:     node.LastSeen,
			DefaultURL:   target.URL,
			DefaultTitle: target.Title,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].HostKey == summaries[j].HostKey {
			if summaries[i].Depth == summaries[j].Depth {
				return summaries[i].Path < summaries[j].Path
			}
			return summaries[i].Depth < summaries[j].Depth
		}
		return summaries[i].HostKey < summaries[j].HostKey
	})
	return summaries
}

func (db *DB) ensureCompatibility() {
	if db.Entries == nil {
		db.Entries = []Entry{}
	}
	if db.Targets == nil {
		db.Targets = []Target{}
	}
	if db.Nodes == nil {
		db.Nodes = []Node{}
	}
	if db.Rules == nil {
		db.Rules = []Rule{}
	}

	for i := range db.Rules {
		db.Rules[i] = normalizeRule(db.Rules[i])
	}

	if len(db.Targets) == 0 && len(db.Entries) > 0 {
		db.migrateLegacyEntries()
	}

	db.rebuildDerivedState()
}

func (db *DB) migrateLegacyEntries() {
	now := time.Now().Unix()
	db.Metadata.MigratedFromEntries = true
	db.Metadata.LegacyEntryCount = len(db.Entries)
	db.Metadata.LastMigratedAt = now

	targets := make([]Target, 0, len(db.Entries))
	for _, entry := range db.Entries {
		if strings.TrimSpace(entry.URL) == "" {
			continue
		}

		canonical, err := db.deriveCanonicalTargetFromNormalized(entry.URL)
		if err != nil {
			canonical, err = db.deriveCanonicalTarget(entry.URL)
			if err != nil {
				continue
			}
		}

		count := entry.Count
		if count <= 0 {
			count = 1
		}
		lastSeen := entry.LastSeen
		if lastSeen <= 0 {
			lastSeen = now
		}

		targets = append(targets, Target{
			Key:      canonical.TargetURL,
			URL:      canonical.TargetURL,
			Title:    strings.TrimSpace(entry.Title),
			Count:    count,
			LastSeen: lastSeen,
			Source:   normalizeSource(entry.Source),
			HostKey:  canonical.HostKey,
			NodePath: canonical.NodePath,
			NodeKey:  canonical.NodeKey,
		})
	}
	db.Targets = targets
}

func (db *DB) rebuildDerivedState() {
	now := time.Now().Unix()
	targetByKey := map[string]*Target{}
	targets := make([]Target, 0, len(db.Targets))

	for _, target := range db.Targets {
		if strings.TrimSpace(target.URL) == "" {
			continue
		}

		canonical, err := db.deriveCanonicalTargetFromNormalized(target.URL)
		if err != nil {
			continue
		}

		target.Key = canonical.TargetURL
		target.URL = canonical.TargetURL
		target.HostKey = canonical.HostKey
		target.NodePath = canonical.NodePath
		target.NodeKey = canonical.NodeKey
		target.Source = normalizeSource(target.Source)
		target.Title = strings.TrimSpace(target.Title)
		if target.Count <= 0 {
			target.Count = 1
		}
		if target.LastSeen <= 0 {
			target.LastSeen = now
		}

		if existing, ok := targetByKey[target.Key]; ok {
			existing.Count += target.Count
			if target.LastSeen > existing.LastSeen {
				existing.LastSeen = target.LastSeen
			}
			if target.Title != "" {
				existing.Title = target.Title
			}
			if existing.Source != SourceManual {
				existing.Source = target.Source
			}
			if target.Source == SourceManual {
				existing.Source = SourceManual
			}
			continue
		}

		targets = append(targets, target)
		targetByKey[target.Key] = &targets[len(targets)-1]
	}

	db.Targets = targets

	nodeMap := map[string]*Node{}
	for i := range db.Targets {
		target := &db.Targets[i]
		chain := buildNodeChain(target.HostKey, target.NodePath)
		for _, ref := range chain {
			node := nodeMap[ref.Key]
			if node == nil {
				node = &Node{
					Key:       ref.Key,
					HostKey:   ref.HostKey,
					Path:      ref.Path,
					ParentKey: ref.ParentKey,
				}
				nodeMap[ref.Key] = node
			}

			node.Count += target.Count
			if target.LastSeen > node.LastSeen {
				node.LastSeen = target.LastSeen
			}
			if current := targetByKey[node.DefaultTargetKey]; current == nil || betterTarget(*target, *current) {
				node.DefaultTargetKey = target.Key
			}
			if target.Source == SourceManual {
				if current := targetByKey[node.ManualTargetKey]; current == nil || betterTarget(*target, *current) {
					node.ManualTargetKey = target.Key
				}
			}
			if ref.Key == target.NodeKey {
				if current := targetByKey[node.ExactTargetKey]; current == nil || betterTarget(*target, *current) {
					node.ExactTargetKey = target.Key
				}
			}
		}
	}

	nodes := make([]Node, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, *node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].HostKey == nodes[j].HostKey {
			if nodeDepth(nodes[i].Path) == nodeDepth(nodes[j].Path) {
				return nodes[i].Path < nodes[j].Path
			}
			return nodeDepth(nodes[i].Path) < nodeDepth(nodes[j].Path)
		}
		return nodes[i].HostKey < nodes[j].HostKey
	})
	db.Nodes = nodes

	db.Metadata.SchemaVersion = schemaVersion
	db.syncEntriesFromTargets()
}

func (db *DB) syncEntriesFromTargets() {
	entries := make([]Entry, 0, len(db.Targets))
	for _, target := range db.Targets {
		entries = append(entries, targetToEntry(target))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastSeen == entries[j].LastSeen {
			return entries[i].URL < entries[j].URL
		}
		return entries[i].LastSeen > entries[j].LastSeen
	})
	db.Entries = entries
}

func (db *DB) resolveAddressMatch(keyword string) (Match, bool) {
	input := normalizeAddressInput(keyword)
	if input == "" {
		return Match{}, false
	}

	input = db.expandAliasInput(input)
	parts := strings.SplitN(input, "/", 2)
	hostToken := parts[0]
	hostKey := db.resolveHostToken(hostToken)
	if hostKey == "" {
		return Match{}, false
	}

	nodeKey := hostKey
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		nodePath := db.canonicalNodePath(hostKey, parts[1])
		nodeKey = buildNodeKey(hostKey, nodePath)
	}

	if db.findNodeByKey(nodeKey) == nil {
		return Match{}, false
	}

	target := db.resolveLandingTarget(nodeKey, map[string]bool{})
	if target == nil {
		return Match{}, false
	}

	return Match{
		Entry:   targetToEntry(*target),
		Score:   1_000_000 + float64(target.Count),
		Reason:  "address",
		NodeKey: nodeKey,
	}, true
}

func (db *DB) resolveLandingTarget(nodeKey string, visited map[string]bool) *Target {
	if nodeKey == "" {
		return nil
	}
	if visited[nodeKey] {
		return nil
	}
	visited[nodeKey] = true

	node := db.findNodeByKey(nodeKey)
	if node == nil {
		return nil
	}

	if ruleTarget := db.resolveDefaultRule(nodeKey, visited); ruleTarget != nil {
		return ruleTarget
	}
	if target := db.findTargetByKey(node.ManualTargetKey); target != nil {
		return target
	}
	if target := db.findTargetByKey(node.DefaultTargetKey); target != nil {
		return target
	}
	if target := db.findTargetByKey(node.ExactTargetKey); target != nil {
		return target
	}
	return nil
}

func (db *DB) resolveDefaultRule(nodeKey string, visited map[string]bool) *Target {
	for _, rule := range db.Rules {
		if rule.Type != RuleDefault {
			continue
		}
		if rule.Pattern != nodeKey {
			continue
		}

		value := strings.TrimSpace(rule.Value)
		if value == "" {
			continue
		}

		if strings.Contains(value, "://") {
			canonical, err := db.deriveCanonicalTarget(value)
			if err != nil {
				continue
			}
			if target := db.findTargetByURL(canonical.TargetURL); target != nil {
				return target
			}
			continue
		}

		if target := db.findTargetByKey(value); target != nil {
			return target
		}
		if node := db.findNodeByKey(value); node != nil {
			return db.resolveLandingTarget(node.Key, visited)
		}
	}
	return nil
}

func (db *DB) deriveCanonicalTarget(rawURL string) (canonicalTarget, error) {
	normalized, err := urlnorm.NormalizeAndRedact(rawURL)
	if err != nil {
		return canonicalTarget{}, err
	}
	return db.deriveCanonicalTargetFromNormalized(normalized)
}

func (db *DB) deriveCanonicalTargetFromNormalized(normalized string) (canonicalTarget, error) {
	u, err := url.Parse(strings.TrimSpace(normalized))
	if err != nil {
		return canonicalTarget{}, err
	}

	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		host = strings.ToLower(strings.TrimSpace(u.Host))
	}
	if host == "" {
		return canonicalTarget{}, fmt.Errorf("invalid host in url: %s", normalized)
	}

	if db.matchesIgnoreRule(host, u.Path, normalized) {
		return canonicalTarget{}, ErrIgnoredByRule
	}

	u.RawQuery = filterQueryValues(u.Query(), db.preservedQueryKeys(host, u.Path))
	targetURL := u.String()
	nodePath := db.canonicalNodePath(host, u.Path)
	chain := buildNodeChain(host, nodePath)
	nodeKey := host
	if len(chain) > 0 {
		nodeKey = chain[len(chain)-1].Key
	}

	return canonicalTarget{
		TargetURL: targetURL,
		HostKey:   host,
		NodePath:  nodePath,
		NodeKey:   nodeKey,
		NodeChain: chain,
	}, nil
}

func (db *DB) canonicalNodePath(host, rawPath string) string {
	segments := splitPathSegments(strings.ToLower(rawPath))
	normalized := make([]string, 0, len(segments))
	for _, segment := range segments {
		normalized = append(normalized, normalizeDynamicSegment(segment))
	}
	path := canonicalPathFromSegments(normalized)
	return db.applyCollapseRules(host, path)
}

func (db *DB) applyCollapseRules(host, path string) string {
	path = normalizeComparablePath(path)
	for _, rule := range db.Rules {
		if rule.Type != RuleCollapse {
			continue
		}
		if rule.Host != "" && rule.Host != host {
			continue
		}
		if matchRulePath(path, rule.Pattern) {
			value := normalizeComparablePath(rule.Value)
			if value != "" {
				return value
			}
		}
	}
	return path
}

func (db *DB) matchesIgnoreRule(host, rawPath, normalized string) bool {
	path := normalizeComparablePath(strings.ToLower(rawPath))
	full := strings.ToLower(strings.TrimSpace(normalized))
	for _, rule := range db.Rules {
		if rule.Type != RuleIgnore {
			continue
		}
		if rule.Host != "" && rule.Host != host {
			continue
		}
		if rule.Pattern == "" {
			return true
		}
		if matchRulePath(path, rule.Pattern) || strings.Contains(full, strings.ToLower(rule.Pattern)) {
			return true
		}
	}
	return false
}

func (db *DB) preservedQueryKeys(host, rawPath string) map[string]struct{} {
	keys := map[string]struct{}{}
	path := normalizeComparablePath(strings.ToLower(rawPath))
	for _, rule := range db.Rules {
		if rule.Type != RulePreserveQuery {
			continue
		}
		if rule.Host != "" && rule.Host != host {
			continue
		}
		if !matchRulePath(path, rule.Pattern) {
			continue
		}
		for _, part := range strings.Split(rule.Value, ",") {
			key := strings.ToLower(strings.TrimSpace(part))
			if key != "" {
				keys[key] = struct{}{}
			}
		}
	}
	return keys
}

func (db *DB) expandAliasInput(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}

	for depth := 0; depth < 8; depth++ {
		changed := false
		for _, rule := range db.Rules {
			if rule.Type != RuleAlias {
				continue
			}
			pattern := strings.ToLower(strings.TrimSpace(rule.Pattern))
			value := strings.ToLower(strings.TrimSpace(rule.Value))
			if pattern == "" || value == "" {
				continue
			}
			switch {
			case input == pattern:
				input = value
				changed = true
			case strings.HasPrefix(input, pattern+"/"):
				input = value + input[len(pattern):]
				changed = true
			}
			if changed {
				break
			}
		}
		if !changed {
			break
		}
	}
	return input
}

func (db *DB) resolveHostToken(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return ""
	}

	hosts := make([]string, 0, len(db.Nodes))
	seen := map[string]struct{}{}
	for _, node := range db.Nodes {
		if node.Path != "" {
			continue
		}
		if _, ok := seen[node.HostKey]; ok {
			continue
		}
		seen[node.HostKey] = struct{}{}
		hosts = append(hosts, node.HostKey)
	}

	for _, host := range hosts {
		if host == token {
			return host
		}
	}

	if strings.Contains(token, ".") {
		return ""
	}

	var exactShort []string
	for _, host := range hosts {
		if shortHostName(host) == token {
			exactShort = append(exactShort, host)
		}
	}
	if len(exactShort) == 1 {
		return exactShort[0]
	}

	var prefix []string
	for _, host := range hosts {
		if strings.HasPrefix(shortHostName(host), token) {
			prefix = append(prefix, host)
		}
	}
	if len(prefix) == 1 {
		return prefix[0]
	}

	var contains []string
	for _, host := range hosts {
		if strings.Contains(host, token) {
			contains = append(contains, host)
		}
	}
	if len(contains) == 1 {
		return contains[0]
	}

	return ""
}

func (db *DB) findTargetByURL(url string) *Target {
	for i := range db.Targets {
		if db.Targets[i].URL == url {
			return &db.Targets[i]
		}
	}
	return nil
}

func (db *DB) findTargetByKey(key string) *Target {
	for i := range db.Targets {
		if db.Targets[i].Key == key {
			return &db.Targets[i]
		}
	}
	return nil
}

func (db *DB) findNodeByKey(key string) *Node {
	for i := range db.Nodes {
		if db.Nodes[i].Key == key {
			return &db.Nodes[i]
		}
	}
	return nil
}

func normalizeRule(rule Rule) Rule {
	rule.Type = strings.ToLower(strings.TrimSpace(rule.Type))
	rule.Host = strings.ToLower(strings.TrimSpace(rule.Host))

	value := strings.TrimSpace(rule.Value)
	switch rule.Type {
	case RuleAlias:
		rule.Pattern = strings.ToLower(strings.TrimSpace(rule.Pattern))
		rule.Value = strings.ToLower(value)
	case RuleCollapse, RuleDefault:
		rule.Pattern = normalizeRulePattern(rule.Pattern)
		if strings.Contains(value, "://") {
			rule.Value = value
		} else {
			rule.Value = normalizeRulePattern(value)
			if rule.Type == RuleDefault && !strings.Contains(rule.Value, "/") && rule.Value != "" && strings.Contains(value, ".") {
				rule.Value = strings.ToLower(strings.TrimSpace(value))
			}
		}
	case RulePreserveQuery:
		rule.Pattern = normalizeRulePattern(rule.Pattern)
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			key := strings.ToLower(strings.TrimSpace(part))
			if key != "" {
				out = append(out, key)
			}
		}
		rule.Value = strings.Join(out, ",")
	default:
		rule.Pattern = normalizeRulePattern(rule.Pattern)
		rule.Value = value
	}
	return rule
}

func normalizeRulePattern(pattern string) string {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return ""
	}
	if strings.Contains(pattern, "://") {
		return pattern
	}
	if strings.Contains(pattern, ".") && !strings.HasPrefix(pattern, "/") && !strings.Contains(pattern, "*") {
		return pattern
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	return strings.TrimSuffix(pattern, "/")
}

func normalizeAddressInput(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(fields, "/"))
}

func fuzzyMatchReason(keyword string, node Node, target Target) (string, float64) {
	if keyword == "" {
		return "", 0
	}

	short := shortHostName(node.HostKey)
	switch {
	case short == keyword || node.HostKey == keyword || node.Key == keyword:
		return "host", 1.5
	case strings.Contains(node.Key, keyword):
		return "node", 1.25
	case strings.Contains(strings.ToLower(target.Title), keyword):
		return "title", 1.0
	case strings.Contains(strings.ToLower(target.URL), keyword):
		return "url", 0.8
	default:
		return "", 0
	}
}

func scoreNode(keywordScore float64, node Node, target Target, now time.Time) float64 {
	daysAgo := now.Sub(time.Unix(node.LastSeen, 0)).Hours() / 24
	if daysAgo < 0 {
		daysAgo = 0
	}
	lambda := math.Ln2 / 14.0
	decay := math.Exp(-lambda * daysAgo)
	base := math.Max(1, float64(node.Count))
	depthBoost := 1 + 0.10*float64(nodeDepth(node.Path))
	return base * decay * keywordScore * depthBoost * sourceWeight(target.Source)
}

func sourceWeight(source string) float64 {
	switch normalizeSource(source) {
	case SourceManual:
		return 1.35
	case SourceAuto:
		return 1.0
	default:
		return 1.05
	}
}

func normalizeSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case SourceManual, SourceAuto, SourceLegacy:
		return strings.ToLower(strings.TrimSpace(source))
	default:
		return SourceLegacy
	}
}

func filterQueryValues(values url.Values, keep map[string]struct{}) string {
	if len(values) == 0 || len(keep) == 0 {
		return ""
	}

	filtered := url.Values{}
	for key, original := range values {
		if _, ok := keep[strings.ToLower(key)]; !ok {
			continue
		}
		filtered[key] = original
	}
	return filtered.Encode()
}

func buildNodeChain(host, path string) []nodeRef {
	path = normalizeComparablePath(path)
	chain := []nodeRef{{
		Key:     host,
		HostKey: host,
		Path:    "",
	}}
	if path == "" {
		return chain
	}

	segments := splitPathSegments(path)
	current := ""
	parent := host
	for _, segment := range segments {
		if current == "" {
			current = "/" + segment
		} else {
			current += "/" + segment
		}
		key := buildNodeKey(host, current)
		chain = append(chain, nodeRef{
			Key:       key,
			HostKey:   host,
			Path:      current,
			ParentKey: parent,
		})
		parent = key
	}
	return chain
}

func buildNodeKey(host, path string) string {
	path = normalizeComparablePath(path)
	if path == "" {
		return host
	}
	return host + path
}

func splitPathSegments(path string) []string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func canonicalPathFromSegments(segments []string) string {
	clean := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment != "" {
			clean = append(clean, segment)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return "/" + strings.Join(clean, "/")
}

func normalizeComparablePath(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}

func normalizeDynamicSegment(segment string) string {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if segment == "" {
		return ""
	}
	if isAllDigits(segment) || looksUUID(segment) {
		return ":id"
	}
	return segment
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func looksUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				return false
			}
		}
	}
	return true
}

func matchRulePath(path, pattern string) bool {
	path = normalizeComparablePath(path)
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return true
	}

	if strings.HasSuffix(pattern, "*") && strings.Count(pattern, "*") == 1 {
		prefix := strings.TrimSuffix(pattern, "*")
		prefix = normalizeComparablePath(prefix)
		return strings.HasPrefix(path, prefix)
	}

	pattern = normalizeComparablePath(pattern)
	if strings.Contains(pattern, "*") {
		patternSegs := splitPathSegments(pattern)
		pathSegs := splitPathSegments(path)
		if len(patternSegs) != len(pathSegs) {
			return false
		}
		for i := range patternSegs {
			if patternSegs[i] != "*" && patternSegs[i] != pathSegs[i] {
				return false
			}
		}
		return true
	}

	if path == pattern {
		return true
	}
	return strings.HasPrefix(path, pattern+"/")
}

func betterTarget(a, b Target) bool {
	if a.LastSeen != b.LastSeen {
		return a.LastSeen > b.LastSeen
	}
	if nodeDepth(a.NodePath) != nodeDepth(b.NodePath) {
		return nodeDepth(a.NodePath) > nodeDepth(b.NodePath)
	}
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	if sourceWeight(a.Source) != sourceWeight(b.Source) {
		return sourceWeight(a.Source) > sourceWeight(b.Source)
	}
	return a.URL < b.URL
}

func targetToEntry(target Target) Entry {
	return Entry{
		URL:      target.URL,
		Title:    target.Title,
		Count:    target.Count,
		LastSeen: target.LastSeen,
		Source:   normalizeSource(target.Source),
	}
}

func shortHostName(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	parts := strings.Split(host, ".")
	return parts[0]
}

func nodeDepth(path string) int {
	return len(splitPathSegments(path))
}

func pathDepth(nodeKey string) int {
	idx := strings.Index(nodeKey, "/")
	if idx == -1 {
		return 0
	}
	return nodeDepth(nodeKey[idx:])
}
