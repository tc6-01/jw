## ADDED Requirements

### Requirement: Auto import records MUST include curation metadata
The system SHALL annotate each browser-history auto import record with curation metadata including source type, host key, depth bucket, topic key, and curation group key.

#### Scenario: Metadata is derived during auto import
- **WHEN** an auto import row is accepted for persistence
- **THEN** the stored record includes `source=auto` and non-empty curation grouping fields

### Requirement: Auto import SHALL keep one representative per curation group
For auto-imported records in the same curation group, the system SHALL retain exactly one representative record for ranking and default jump resolution.

#### Scenario: Newer record replaces representative in same group
- **WHEN** a newly imported auto record belongs to an existing curation group and is newer than the current representative
- **THEN** the system marks the new record as representative and demotes or removes the previous representative according to retention policy

### Requirement: Different depth buckets MUST remain independently represented
The curation process MUST NOT collapse records across different depth buckets, even when host and topic key are identical.

#### Scenario: Same host and topic but different depth buckets
- **WHEN** two auto records share host and topic key but belong to different depth buckets
- **THEN** the system keeps independent representation for both buckets

### Requirement: Manual records MUST be protected from auto curation replacement
Auto curation MUST NOT overwrite or demote manual records when resolving representative records for auto-import groups.

#### Scenario: Manual and auto overlap in semantic group
- **WHEN** a manual record and an auto record correspond to the same host/topic/depth semantic area
- **THEN** the manual record remains preserved with manual source semantics and is not replaced by auto curation logic

### Requirement: Curation metadata SHALL survive reload and restart
Stored curation metadata SHALL be persisted and restored consistently across process restarts.

#### Scenario: Service restart after auto import
- **WHEN** the service restarts after curation metadata has been written
- **THEN** subsequent queries and imports use restored curation metadata without recomputing inconsistent grouping for existing records
