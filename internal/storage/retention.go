package storage

import (
	"context"
	"errors"
	"time"
)

// RetentionStats reports only the raw evidence rows removed by PruneRetention.
// Durable review, session, motivation and configuration tables are deliberately
// outside this operation.
type RetentionStats struct {
	RawMessagesDeleted      int64
	RawTurnsDeleted         int64
	RawConversationsDeleted int64
	SemanticDeleted         int64
}

// PruneRetention removes expired raw chat and semantic evidence in one
// transaction. observed_at is the retention clock for both data classes;
// ingested_at is intentionally not used because delayed collection must not
// extend a record's lifetime.
func (s *Storage) PruneRetention(ctx context.Context, now time.Time, rawChatDays, semanticDays int) (RetentionStats, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if rawChatDays < 0 || semanticDays < 0 {
		return RetentionStats{}, errors.New("retention days cannot be negative")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RetentionStats{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var stats RetentionStats
	if rawChatDays > 0 {
		cutoff := now.Add(-time.Duration(rawChatDays) * 24 * time.Hour)
		result, err := tx.ExecContext(ctx, `DELETE FROM chat_messages
			WHERE turn_id IN (SELECT id FROM chat_turns WHERE observed_at < ?)`, cutoff)
		if err != nil {
			return RetentionStats{}, err
		}
		if stats.RawMessagesDeleted, err = result.RowsAffected(); err != nil {
			return RetentionStats{}, err
		}

		result, err = tx.ExecContext(ctx, `DELETE FROM chat_turns WHERE observed_at < ?`, cutoff)
		if err != nil {
			return RetentionStats{}, err
		}
		if stats.RawTurnsDeleted, err = result.RowsAffected(); err != nil {
			return RetentionStats{}, err
		}

		result, err = tx.ExecContext(ctx, `DELETE FROM chat_conversations
			WHERE last_seen_at < ?
			AND NOT EXISTS (SELECT 1 FROM chat_turns WHERE conversation_id = chat_conversations.id)`, cutoff)
		if err != nil {
			return RetentionStats{}, err
		}
		if stats.RawConversationsDeleted, err = result.RowsAffected(); err != nil {
			return RetentionStats{}, err
		}
	}

	if semanticDays > 0 {
		cutoff := now.Add(-time.Duration(semanticDays) * 24 * time.Hour)
		result, err := tx.ExecContext(ctx, `DELETE FROM semantic_snapshots WHERE observed_at < ?`, cutoff)
		if err != nil {
			return RetentionStats{}, err
		}
		if stats.SemanticDeleted, err = result.RowsAffected(); err != nil {
			return RetentionStats{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return RetentionStats{}, err
	}
	return stats, nil
}
