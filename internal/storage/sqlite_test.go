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
