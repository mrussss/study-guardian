package storage

import (
	"context"
	"testing"
	"time"
)

func TestStorageLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// Test Session
	session := SessionRecord{
		ID:              "sess-1",
		Mode:            "STUDY",
		Task:            "Go Lab",
		StartedAt:       now,
		DurationSeconds: 120,
	}
	if err := store.SaveSession(ctx, session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Update Session
	end := now.Add(2 * time.Minute)
	session.EndedAt = &end
	session.EndReason = "USER_BREAK"
	if err := store.SaveSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	// Test Observation
	obs := ObservationRecord{
		Timestamp:   now,
		Interaction: "ACTIVE",
		Relation:    "FOCUSED",
		Privacy:     "NORMAL",
		Confidence:  0.95,
		Reason:      "VSCode focused on Go project",
		CurrentMode: "STUDY",
		Task:        "Go Lab",
	}
	if err := store.RecordObservation(ctx, obs); err != nil {
		t.Fatalf("failed to record observation: %v", err)
	}

	// Test Daily State
	if err := store.UpdateDailyState(ctx, "2026-09-02", 300, 1200, 300, 0, 1500, now); err != nil {
		t.Fatalf("failed to update daily state: %v", err)
	}

	// Test Classification Cache
	cacheKey := "code.exe|main.go|github.com|Go Lab|hash123"
	err = store.SetClassificationCache(ctx, cacheKey, "FOCUSED", 0.98, "Matched code editor", now, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("failed to set classification cache: %v", err)
	}

	rel, conf, reason, found := store.GetClassificationCache(ctx, cacheKey, now.Add(1*time.Minute))
	if !found || rel != "FOCUSED" || conf != 0.98 || reason != "Matched code editor" {
		t.Fatalf("cache lookup failed, got (%v, %s, %f, %s)", found, rel, conf, reason)
	}

	// Expired cache lookup
	_, _, _, found = store.GetClassificationCache(ctx, cacheKey, now.Add(15*time.Minute))
	if found {
		t.Fatalf("expected expired cache entry to not be found")
	}

	// Test Feedback
	if err := store.RecordFeedback(ctx, "rem-1", "ACTUALLY_STUDYING", now); err != nil {
		t.Fatalf("failed to record feedback: %v", err)
	}
}

func TestSemanticSnapshotHasStableDatabaseID(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	id, err := store.RecordSemanticSnapshot(context.Background(), SemanticSnapshotRecord{ObservedAt: now, LocalDate: "2026-09-03", Task: "Go", App: "Code.exe", Title: "main.go", Relation: "FOCUSED", Confidence: .95, Activity: "CODING", SourceKind: "LOCAL_RULE"})
	if err != nil || id <= 0 {
		t.Fatalf("insert id=%d err=%v", id, err)
	}
	rows, err := store.ListSemanticSnapshotsForDate(context.Background(), "2026-09-03")
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestIngestChatTurnIsIdempotentAndKeepsEligibilityFrozen(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// An unnamed fixed offset matches time.Parse(RFC3339) on a host in a
	// different timezone and previously made SQLite return a raw string.
	observed := time.Date(2026, 9, 3, 23, 58, 0, 0, time.FixedZone("", 8*60*60))
	turn := ChatTurnRecord{TurnKey: "turn-1", ObservedAt: observed, LocalDate: "2026-09-03", ModeAtStart: "STUDY", TaskAtStart: "Go", EligibleForReview: true, Finalized: false}
	conversation := ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "conversation-1", ObservedAt: observed}
	message := ChatMessageRecord{ExternalMessageID: "message-1", Role: "user", Content: "Explain interfaces", IsActive: true}
	if _, err := store.IngestChatTurn(context.Background(), conversation, turn, []ChatMessageRecord{message}, observed.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// A retry must update the raw message but must not turn the original STUDY
	// eligibility into BREAK after a later mode change.
	turn.ModeAtStart = "BREAK"
	turn.EligibleForReview = false
	message.Content = "Explain interfaces clearly"
	if _, err := store.IngestChatTurn(context.Background(), conversation, turn, []ChatMessageRecord{message}, observed.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var eligible, messageCount int
	if err := store.db.QueryRow(`SELECT eligible_for_review FROM chat_turns WHERE turn_key = 'turn-1'`).Scan(&eligible); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM chat_messages WHERE external_message_id = 'message-1'`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if eligible != 1 || messageCount != 1 {
		t.Fatalf("eligible=%d messages=%d, want 1/1", eligible, messageCount)
	}
	loaded, err := store.LoadChatTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ObservedAt.Equal(observed) || loaded.ObservedAt.Location() != time.UTC {
		t.Fatalf("observed_at=%v location=%v, want UTC representation of %v", loaded.ObservedAt, loaded.ObservedAt.Location(), observed)
	}
}

func TestRestartDurationRepairIsIdempotent(t *testing.T) {
	path := t.TempDir() + "/repair.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	ended := started.Add(2 * time.Minute)
	if err := store.SaveSession(ctx, SessionRecord{ID: "old", Mode: "STUDY", Task: "Go", StartedAt: started, EndedAt: &ended, DurationSeconds: 120, EndReason: "USER_BREAK"}); err != nil {
		t.Fatal(err)
	}
	current := ended.Add(2 * time.Second)
	newEnded := current.Add(2 * time.Second)
	if err := store.SaveSession(ctx, SessionRecord{ID: "new", Mode: "STUDY", Task: "Go", StartedAt: current, EndedAt: &newEnded, DurationSeconds: 120, EndReason: "RESTART_RECOVERY"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDailyState(ctx, "2026-09-08", 0, 120, 0, 0, 0, current); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE sessions SET started_at = ?, ended_at = ? WHERE id = 'old'`, canonicalDBTime(started), canonicalDBTime(ended)); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	var duration int64
	if err := store.db.QueryRow(`SELECT duration_seconds FROM sessions WHERE id = 'new'`).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 0 {
		t.Fatalf("repaired duration=%d, want 0", duration)
	}
	var original, repaired int64
	var reason string
	var version int
	if err := store.db.QueryRow(`SELECT original_duration_seconds, repaired_duration_seconds, repair_reason, repair_version FROM session_duration_repairs WHERE session_id = 'new'`).Scan(&original, &repaired, &reason, &version); err != nil {
		t.Fatal(err)
	}
	if original != 120 || repaired != 0 || reason != "daily_state_restart_inheritance_mismatch" || version != currentRestartRepairVersion {
		t.Fatalf("repair audit=(%d,%d,%q,%d)", original, repaired, reason, version)
	}
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.db.QueryRow(`SELECT duration_seconds FROM sessions WHERE id = 'new'`).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 0 {
		t.Fatalf("second migration changed repaired duration=%d", duration)
	}
	var repairCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM session_duration_repairs WHERE session_id = 'new'`).Scan(&repairCount); err != nil {
		t.Fatal(err)
	}
	if repairCount != 1 {
		t.Fatalf("repair count=%d, want 1", repairCount)
	}
}

func TestSessionListingSortsMixedTimezoneTextByInstant(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.db.Exec(`INSERT INTO sessions (id, mode, task, started_at, local_date, duration_seconds, end_reason)
		VALUES
		('later-utc', 'STUDY', 'Go', '2026-09-08 12:00:00.000000000 +0000 UTC', '2026-09-08', 10, ''),
		('earlier-cst', 'STUDY', 'Go', '2026-09-08 19:59:59.000000000 +0800 CST', '2026-09-08', 10, '')`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListSessionsForDate(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "earlier-cst" || rows[1].ID != "later-utc" {
		t.Fatalf("mixed-timezone order=%+v", rows)
	}
	last, err := store.LoadLastSession(context.Background())
	if err != nil || last.ID != "later-utc" {
		t.Fatalf("last=%+v err=%v", last, err)
	}
}

func seedRestartCandidate(t *testing.T, store *Storage, date, prefix string, offset, normalSeconds, restartSeconds int64) {
	t.Helper()
	started, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	started = started.Add(10*time.Hour + time.Duration(offset)*time.Second)
	normalEnded := started.Add(time.Duration(normalSeconds) * time.Second)
	if err := store.SaveSession(context.Background(), SessionRecord{
		ID: "normal-" + prefix, Mode: "STUDY", Task: "Go", StartedAt: started, LocalDate: date,
		EndedAt: &normalEnded, DurationSeconds: normalSeconds, EndReason: "USER_BREAK",
	}); err != nil {
		t.Fatal(err)
	}
	restartStarted := normalEnded.Add(2 * time.Second)
	restartEnded := restartStarted.Add(2 * time.Second)
	if err := store.SaveSession(context.Background(), SessionRecord{
		ID: "restart-" + prefix, Mode: "STUDY", Task: "Go", StartedAt: restartStarted, LocalDate: date,
		EndedAt: &restartEnded, DurationSeconds: restartSeconds, EndReason: "RESTART_RECOVERY",
	}); err != nil {
		t.Fatal(err)
	}
}

func seedReadyReview(t *testing.T, store *Storage, date string, revision int64) {
	t.Helper()
	for current := int64(0); current < revision; current++ {
		if _, err := store.BumpEvidenceRevision(context.Background(), date, time.Date(2026, 9, 9, 1, 0, int(current), 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 9, 1, 1, 0, 0, time.UTC)
	if err := store.SaveDailyReview(context.Background(), DailyReviewRecord{
		Date: date, Status: "READY", GenerationMode: "FALLBACK", Revision: 7,
		GeneratedEvidenceRevision: revision, InputHash: "old-hash", SchemaVersion: 1,
		ReviewJSON: "old-review", Markdown: "old-markdown", AttemptCount: 1, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRestartRepairInvalidatesReviewAndBumpsRevision(t *testing.T) {
	path := t.TempDir() + "/repair-review.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	seedRestartCandidate(t, store, "2026-09-08", "single", 0, 120, 120)
	if err := store.UpdateDailyState(context.Background(), "2026-09-08", 0, 120, 0, 0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	seedReadyReview(t, store, "2026-09-08", 4)
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var duration int64
	if err := store.db.QueryRow(`SELECT duration_seconds FROM sessions WHERE id = 'restart-single'`).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 0 {
		t.Fatalf("duration=%d, want 0", duration)
	}
	revision, err := store.GetEvidenceRevision(context.Background(), "2026-09-08")
	if err != nil || revision != 5 {
		t.Fatalf("revision=%d err=%v, want 5", revision, err)
	}
	review, err := store.LoadDailyReview(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "STALE" || review.ReviewJSON != "old-review" || review.GeneratedEvidenceRevision != 4 {
		t.Fatalf("review=%+v", review)
	}
}

func TestRestartRepairBumpsSameDateOnlyOnceForMultipleSessions(t *testing.T) {
	path := t.TempDir() + "/repair-same-date.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	seedRestartCandidate(t, store, "2026-09-08", "a", 0, 60, 100)
	seedRestartCandidate(t, store, "2026-09-08", "b", 300, 60, 100)
	if err := store.UpdateDailyState(context.Background(), "2026-09-08", 0, 120, 0, 0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	seedReadyReview(t, store, "2026-09-08", 2)
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	revision, err := store.GetEvidenceRevision(context.Background(), "2026-09-08")
	if err != nil || revision != 3 {
		t.Fatalf("revision=%d err=%v, want 3", revision, err)
	}
	var repairs int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM session_duration_repairs WHERE session_id LIKE 'restart-%'`).Scan(&repairs); err != nil {
		t.Fatal(err)
	}
	if repairs != 2 {
		t.Fatalf("repairs=%d, want 2", repairs)
	}
}

func TestRestartRepairBumpsEachAffectedDateAndStalesEachReview(t *testing.T) {
	path := t.TempDir() + "/repair-cross-date.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-09-07", "2026-09-08"} {
		seedRestartCandidate(t, store, date, date, 0, 60, 100)
		if err := store.UpdateDailyState(context.Background(), date, 0, 60, 0, 0, 0, time.Now()); err != nil {
			t.Fatal(err)
		}
		seedReadyReview(t, store, date, 3)
	}
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, date := range []string{"2026-09-07", "2026-09-08"} {
		revision, err := store.GetEvidenceRevision(context.Background(), date)
		if err != nil || revision != 4 {
			t.Fatalf("date=%s revision=%d err=%v, want 4", date, revision, err)
		}
		review, err := store.LoadDailyReview(context.Background(), date)
		if err != nil {
			t.Fatal(err)
		}
		if review.Status != "STALE" {
			t.Fatalf("date=%s review=%+v", date, review)
		}
	}
}

func TestRestartRepairIsIdempotentForRevisionAndReview(t *testing.T) {
	path := t.TempDir() + "/repair-idempotent.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	seedRestartCandidate(t, store, "2026-09-08", "idempotent", 0, 120, 120)
	if err := store.UpdateDailyState(context.Background(), "2026-09-08", 0, 120, 0, 0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	seedReadyReview(t, store, "2026-09-08", 2)
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	firstRevision, err := store.GetEvidenceRevision(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	firstAudit, err := countRepairRows(store)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secondRevision, err := store.GetEvidenceRevision(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	secondAudit, err := countRepairRows(store)
	if err != nil {
		t.Fatal(err)
	}
	review, err := store.LoadDailyReview(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if firstRevision != 3 || secondRevision != firstRevision || firstAudit != 1 || secondAudit != firstAudit || review.Status != "STALE" {
		t.Fatalf("first=(revision=%d audit=%d) second=(revision=%d audit=%d) review=%s", firstRevision, firstAudit, secondRevision, secondAudit, review.Status)
	}
}

func TestRestartRepairLeavesUncertainCandidateAndReviewUntouched(t *testing.T) {
	path := t.TempDir() + "/repair-uncertain.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	seedRestartCandidate(t, store, "2026-09-08", "uncertain", 0, 100, 100)
	if err := store.UpdateDailyState(context.Background(), "2026-09-08", 0, 50, 0, 0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	seedReadyReview(t, store, "2026-09-08", 2)
	_ = store.Close()

	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var duration int64
	if err := store.db.QueryRow(`SELECT duration_seconds FROM sessions WHERE id = 'restart-uncertain'`).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	revision, err := store.GetEvidenceRevision(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	review, err := store.LoadDailyReview(context.Background(), "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	repairs, err := countRepairRows(store)
	if err != nil {
		t.Fatal(err)
	}
	if duration != 100 || revision != 2 || repairs != 0 || review.Status != "READY" {
		t.Fatalf("duration=%d revision=%d repairs=%d review=%s", duration, revision, repairs, review.Status)
	}
}

func countRepairRows(store *Storage) (int, error) {
	var count int
	err := store.db.QueryRow(`SELECT COUNT(*) FROM session_duration_repairs`).Scan(&count)
	return count, err
}
