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
is used only to calculate freshness. A stale, unavailable, sensitive, or
unknown observation becomes live `UNKNOWN` and is never persisted.

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

## Verification

```text
go test ./...                         PASS
git diff --check                       PASS
```

Coverage includes deterministic activity mapping, relation orthogonality,
privacy and freshness fail-soft behavior, stable transitions, title-key
exclusion, heartbeat throttling, cross-midnight local dates, database IDs,
aggregator evidence references, and the authenticated API contract.
