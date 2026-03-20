## ADDED Requirements

### Requirement: Manual source SHALL have higher base priority than auto source
For equivalent keyword relevance and freshness conditions, manual records SHALL rank above auto records.

#### Scenario: Manual and auto both match same keyword
- **WHEN** query results include at least one manual candidate and one auto candidate with comparable textual match quality
- **THEN** the highest-ranked manual candidate appears above the auto candidate

### Requirement: Auto source MAY dominate when manual does not strongly match
When no manual candidate provides a strong or direct match, the system SHALL allow auto candidates to win based on behavior score and recency.

#### Scenario: No strong manual candidate
- **WHEN** manual candidates are absent or weakly matched for a query
- **THEN** an eligible auto candidate can be selected as top result

### Requirement: Jump default target SHALL be the most recent deep representative page
The jump resolver SHALL choose the most recently seen representative deep page among top eligible candidates for the resolved target.

#### Scenario: Multiple eligible deep-page candidates
- **WHEN** the top candidate set includes multiple deep-page representatives in the same semantic intent
- **THEN** jump resolves to the one with the most recent activity timestamp

### Requirement: CLI jump and server jump endpoint MUST share one resolution policy
`jw jump` and `GET /jump` SHALL apply the same source-priority and deep-page resolution rules.

#### Scenario: Same query through CLI and HTTP API
- **WHEN** a user executes `jw jump <keyword>` and another client calls `GET /jump?q=<keyword>` against the same data snapshot
- **THEN** both return the same resolved destination URL
