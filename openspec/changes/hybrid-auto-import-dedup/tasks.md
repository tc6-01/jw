## 1. Data Model and Compatibility

- [x] 1.1 Extend local store entry schema with source and curation metadata fields (`source`, `group_key`, `depth_bucket`, `topic_key`, representative marker)
- [x] 1.2 Add compatibility loading logic that maps pre-change records to `legacy` source semantics without data loss
- [x] 1.3 Add persistence tests for mixed `manual` / `auto` / `legacy` entries to verify round-trip stability

## 2. Curation Key Derivation

- [x] 2.1 Implement deterministic extraction for host key, depth bucket, and topic key from normalized URL
- [x] 2.2 Implement curation group key builder (`host + depth-bucket + topic-key`) with unit tests
- [x] 2.3 Implement representative replacement rule for auto records within one group (newer replaces older)

## 3. Auto Import Curation Flow

- [x] 3.1 Integrate curation grouping into browser-history import path before writing records
- [x] 3.2 Enforce one representative auto record per curation group while preserving records in different depth buckets
- [x] 3.3 Ensure auto curation never overwrites or demotes manual entries
- [x] 3.4 Persist curation metadata updates and verify restart consistency

## 4. Ranking and Jump Resolution

- [x] 4.1 Introduce source-priority weighting in query scoring (`manual > auto`, `legacy` neutral)
- [x] 4.2 Implement default jump target selection to prefer most recent deep representative page
- [x] 4.3 Refactor resolver usage so CLI `jw jump` and HTTP `GET /jump` share identical resolution logic

## 5. Migration and Documentation

- [x] 5.1 Document migration behavior for legacy records and rollback strategy
- [x] 5.2 Update README command semantics for auto-import curation and jump priority behavior
- [x] 5.3 Add operator notes for tuning depth/topic grouping thresholds if needed

## 6. Verification and Regression Safety

- [x] 6.1 Add spec-aligned tests for curation requirements (grouping, representative uniqueness, manual protection)
- [x] 6.2 Add spec-aligned tests for jump priority requirements (manual precedence, auto dominance when manual weak, deep-page default)
- [x] 6.3 Run full regression (`go test ./...`) and compare noisy-domain query outcomes before/after
