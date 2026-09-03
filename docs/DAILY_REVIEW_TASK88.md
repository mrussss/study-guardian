# Task 88 — Semantic snapshots

Task 88 adds a deterministic local semantic layer to the existing Supervisor
tick. It consumes the already fetched ActivityWatch snapshot plus the existing
`TickOutcome`, task, interaction, relation, and privacy state. It does not add
an ActivityWatch query, screenshot, Vision request, or Text AI request.

## Contract

`GET /v1/activity/current` is protected by the main Supervisor Bearer token
and returns `CurrentActivityView` with `schema_version`, `observed_at`,
`fresh`, `user_mode`, `task`, `interaction`, `relation`, `privacy`,
`activity`, and `confidence`.

The activity enum is:

`CODING`, `ALGORITHM`, `READING`, `WRITING`, `WATCHING`, `AI_ASSISTED`,
`BROWSING`, `GENERAL_STUDY`, `UNKNOWN`.

`observed_at` is the Supervisor observation time. ActivityWatch's event time
and positive finite heartbeat `duration` form an effective event end
(`timestamp + duration`); freshness is calculated from that end, not from a
second wall clock. A future timestamp is stale, and invalid duration values
are treated as zero. The Supervisor live view uses the existing 2-minute
window; Task 88's ActivityWatch rule remains 10 seconds. A stale, unavailable,
sensitive, or unknown observation becomes live `UNKNOWN` and is never
persisted.

## Persistence and evidence

Only `STUDY` observations with normal privacy, fresh ActivityWatch data, and a
known local semantic activity are recorded. `DISTRACTED` observations may be
recorded, but their fallback wording explicitly says that they describe an
observed activity and do not prove effective focus or mastery.

The recorder uses a 6-second stable-transition gate, a 15-second minimum
transition interval, and a 180-second heartbeat. The semantic key excludes
the full title and contains task, relation, activity, interaction, and
normalized app/domain. Database IDs are used for stable `semantic:<id>`
evidence references.

## P1.1 boundary notes

`semantic.fresh` describes the semantic snapshot, not whether the Supervisor
transport is reachable. Pet consumers receive a separate `connected` signal:
connected plus stale is a neutral `IDLE`, while disconnected is `OFFLINE`.
Sensitive observations keep `privacy=SENSITIVE` but expose
`fresh=false`, `activity=UNKNOWN`, and `confidence=0` without persistence.

## Verification

```text
go test ./...                         PASS
git diff --check                       PASS
```

Coverage includes deterministic activity mapping, relation orthogonality,
privacy and freshness fail-soft behavior, stable transitions, title-key
exclusion, heartbeat throttling, cross-midnight local dates, database IDs,
aggregator evidence references, and the authenticated API contract.
