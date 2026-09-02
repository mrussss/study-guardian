package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type MotivationDaily struct {
	Date                 string    `json:"date"`
	CreditedFocusSeconds int64     `json:"credited_focus_seconds"`
	DailyTargetSeconds   int64     `json:"daily_target_seconds"`
	CheckinCompleted     bool      `json:"checkin_completed"`
	TargetCompleted      bool      `json:"target_completed"`
	UpdatedAt            time.Time `json:"updated_at"`
}
type Mission struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	RewardMilliAP int64      `json:"reward_milli_ap"`
	DueDate       *string    `json:"due_date,omitempty"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}
type Reward struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	CostMilliAP int64  `json:"cost_milli_ap"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}
type Redemption struct {
	ID          string    `json:"id"`
	RewardID    string    `json:"reward_id"`
	RewardName  string    `json:"reward_name"`
	CostMilliAP int64     `json:"cost_milli_ap"`
	RedeemedAt  time.Time `json:"redeemed_at"`
}
type Achievement struct {
	ID           string    `json:"achievement_id"`
	UnlockedAt   time.Time `json:"unlocked_at"`
	MetadataJSON string    `json:"metadata_json"`
}
type UIEvent struct {
	ID           int64     `json:"id"`
	EventType    string    `json:"event_type"`
	Message      string    `json:"message"`
	MetadataJSON string    `json:"metadata_json"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Storage) RecordCreditedFocus(ctx context.Context, date string, delta, targetSeconds, checkinSeconds int64, now time.Time) (bool, bool, error) {
	if delta <= 0 {
		return false, false, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()
	var before int64
	var oldCheckin, oldTarget bool
	if err = tx.QueryRowContext(ctx, `SELECT credited_focus_seconds, checkin_completed, target_completed FROM motivation_daily WHERE date = ?`, date).Scan(&before, &oldCheckin, &oldTarget); err == sql.ErrNoRows {
		before = 0
	} else if err != nil {
		return false, false, err
	}
	if targetSeconds <= 0 {
		targetSeconds = 7200
	}
	if checkinSeconds <= 0 {
		checkinSeconds = 1800
	}
	newTotal := before + delta
	newCheckin := oldCheckin || newTotal >= checkinSeconds
	newTarget := oldTarget || newTotal >= targetSeconds
	_, err = tx.ExecContext(ctx, `INSERT INTO motivation_daily(date,credited_focus_seconds,daily_target_seconds,checkin_completed,target_completed,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(date) DO UPDATE SET credited_focus_seconds=excluded.credited_focus_seconds,daily_target_seconds=excluded.daily_target_seconds,checkin_completed=excluded.checkin_completed,target_completed=excluded.target_completed,updated_at=excluded.updated_at`, date, newTotal, targetSeconds, newCheckin, newTarget, now)
	if err != nil {
		return false, false, err
	}
	if err = tx.Commit(); err != nil {
		return false, false, err
	}
	return !oldCheckin && newCheckin, !oldTarget && newTarget, nil
}

func (s *Storage) GetMotivationDaily(ctx context.Context, date string) (MotivationDaily, error) {
	var d MotivationDaily
	err := s.db.QueryRowContext(ctx, `SELECT date,credited_focus_seconds,daily_target_seconds,checkin_completed,target_completed,updated_at FROM motivation_daily WHERE date=?`, date).Scan(&d.Date, &d.CreditedFocusSeconds, &d.DailyTargetSeconds, &d.CheckinCompleted, &d.TargetCompleted, &d.UpdatedAt)
	return d, err
}

func (s *Storage) GetMotivationTarget(ctx context.Context, defaultSeconds int64, now time.Time) (int64, error) {
	if defaultSeconds <= 0 {
		defaultSeconds = 7200
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO motivation_settings(id,daily_target_seconds,updated_at) VALUES(1,?,?)`, defaultSeconds, now)
	if err != nil {
		return 0, err
	}
	var target int64
	err = s.db.QueryRowContext(ctx, `SELECT daily_target_seconds FROM motivation_settings WHERE id=1`).Scan(&target)
	return target, err
}

func (s *Storage) SetMotivationTarget(ctx context.Context, targetSeconds int64, date string, now time.Time) error {
	if targetSeconds <= 0 {
		return fmt.Errorf("daily target must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO motivation_settings(id,daily_target_seconds,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET daily_target_seconds=excluded.daily_target_seconds,updated_at=excluded.updated_at`, targetSeconds, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE motivation_daily SET daily_target_seconds=?, target_completed=(credited_focus_seconds>=?), updated_at=? WHERE date=?`, targetSeconds, targetSeconds, now, date); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Storage) APSummaryForDate(ctx context.Context, date string, apPerFocusHourMilli int64) (earned, spent int64, err error) {
	if apPerFocusHourMilli <= 0 {
		apPerFocusHourMilli = 1000
	}
	var focus int64
	if err = s.db.QueryRowContext(ctx, `SELECT COALESCE(credited_focus_seconds,0) FROM motivation_daily WHERE date=?`, date).Scan(&focus); err == sql.ErrNoRows {
		err = nil
	} else if err != nil {
		return 0, 0, err
	}
	var positive, negative int64
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN delta_milli_ap>0 THEN delta_milli_ap ELSE 0 END),0), COALESCE(SUM(CASE WHEN delta_milli_ap<0 THEN -delta_milli_ap ELSE 0 END),0) FROM ap_ledger WHERE date(created_at)=?`, date).Scan(&positive, &negative)
	if err != nil {
		return 0, 0, err
	}
	return focus*apPerFocusHourMilli/3600 + positive, negative, nil
}
func (s *Storage) ListMotivationDaily(ctx context.Context, fromDate string) ([]MotivationDaily, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date,credited_focus_seconds,daily_target_seconds,checkin_completed,target_completed,updated_at FROM motivation_daily WHERE date>=? ORDER BY date DESC`, fromDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MotivationDaily
	for rows.Next() {
		var d MotivationDaily
		if err := rows.Scan(&d.Date, &d.CreditedFocusSeconds, &d.DailyTargetSeconds, &d.CheckinCompleted, &d.TargetCompleted, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Storage) TotalCreditedFocus(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(credited_focus_seconds),0) FROM motivation_daily`).Scan(&n)
	return n, err
}
func (s *Storage) SumAPLedger(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(delta_milli_ap),0) FROM ap_ledger`).Scan(&n)
	return n, err
}

func (s *Storage) RecordUIEvent(ctx context.Context, eventType, message, metadata string, now time.Time) (UIEvent, error) {
	if metadata == "" {
		metadata = "{}"
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO ui_events(event_type,message,metadata_json,created_at) VALUES(?,?,?,?)`, eventType, message, metadata, now)
	if err != nil {
		return UIEvent{}, err
	}
	id, err := res.LastInsertId()
	return UIEvent{ID: id, EventType: eventType, Message: message, MetadataJSON: metadata, CreatedAt: now}, err
}

func (s *Storage) ListUIEvents(ctx context.Context, afterID int64, limit int) ([]UIEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,event_type,message,metadata_json,created_at FROM ui_events WHERE id>? ORDER BY id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UIEvent, 0)
	for rows.Next() {
		var e UIEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.Message, &e.MetadataJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Storage) PruneUIEvents(ctx context.Context, before time.Time, maxRows int) error {
	if maxRows <= 0 {
		maxRows = 1000
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM ui_events WHERE created_at<? OR id NOT IN (SELECT id FROM ui_events ORDER BY id DESC LIMIT ?)`, before, maxRows)
	return err
}
func (s *Storage) CheckinDates(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date FROM motivation_daily WHERE checkin_completed=1 ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Storage) ListMissions(ctx context.Context) ([]Mission, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,title,description,reward_milli_ap,due_date,status,created_at,completed_at FROM missions ORDER BY CASE WHEN status='OPEN' THEN 0 ELSE 1 END, COALESCE(due_date,'9999-12-31'),created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Mission, 0)
	for rows.Next() {
		var m Mission
		if err := rows.Scan(&m.ID, &m.Title, &m.Description, &m.RewardMilliAP, &m.DueDate, &m.Status, &m.CreatedAt, &m.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Storage) CreateMission(ctx context.Context, m Mission) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO missions(id,title,description,reward_milli_ap,due_date,status,created_at) VALUES(?,?,?,?,?,'OPEN',?)`, m.ID, m.Title, m.Description, m.RewardMilliAP, m.DueDate, m.CreatedAt)
	return err
}
func (s *Storage) CompleteMission(ctx context.Context, id string, now time.Time) (Mission, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Mission{}, false, err
	}
	defer tx.Rollback()
	var m Mission
	err = tx.QueryRowContext(ctx, `SELECT id,title,description,reward_milli_ap,due_date,status,created_at,completed_at FROM missions WHERE id=?`, id).Scan(&m.ID, &m.Title, &m.Description, &m.RewardMilliAP, &m.DueDate, &m.Status, &m.CreatedAt, &m.CompletedAt)
	if err == sql.ErrNoRows {
		return Mission{}, false, fmt.Errorf("mission not found")
	}
	if err != nil {
		return Mission{}, false, err
	}
	if m.Status != "OPEN" {
		return m, false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE missions SET status='COMPLETED',completed_at=? WHERE id=? AND status='OPEN'`, now, id); err != nil {
		return Mission{}, false, err
	}
	if m.RewardMilliAP > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO ap_ledger(id,source,reference_id,delta_milli_ap,created_at) VALUES(?,?,?,?,?)`, fmt.Sprintf("mission-%s", id), "MISSION", id, m.RewardMilliAP, now); err != nil {
			return Mission{}, false, err
		}
	}
	m.Status = "COMPLETED"
	m.CompletedAt = &now
	if err = tx.Commit(); err != nil {
		return Mission{}, false, err
	}
	return m, true, nil
}
func (s *Storage) CancelMission(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE missions SET status='CANCELLED' WHERE id=? AND status='OPEN'`, id)
	if err == nil {
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("mission not found or already closed")
		}
	}
	return err
}

func (s *Storage) ListAchievements(ctx context.Context) ([]Achievement, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT achievement_id,unlocked_at,metadata_json FROM achievements ORDER BY unlocked_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Achievement, 0)
	for rows.Next() {
		var a Achievement
		if err := rows.Scan(&a.ID, &a.UnlockedAt, &a.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Storage) UnlockAchievement(ctx context.Context, id string, now time.Time, metadata string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO achievements(achievement_id,unlocked_at,metadata_json) VALUES(?,?,?)`, id, now, metadata)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
func (s *Storage) MissionCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM missions WHERE status='COMPLETED'`).Scan(&n)
	return n, err
}

func (s *Storage) HasDistractionBefore(ctx context.Context, before time.Time) (bool, error) {
	var found bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM observations WHERE relation='DISTRACTED' AND timestamp < ?)`, before).Scan(&found)
	return found, err
}

func (s *Storage) ListRewards(ctx context.Context) ([]Reward, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,type,cost_milli_ap,description,enabled FROM reward_catalog WHERE enabled=1 ORDER BY cost_milli_ap,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reward
	for rows.Next() {
		var r Reward
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.CostMilliAP, &r.Description, &r.Enabled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Storage) RedeemReward(ctx context.Context, id string, now time.Time, apPerFocusHourMilli int64) (Redemption, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Redemption{}, err
	}
	defer tx.Rollback()
	var r Reward
	err = tx.QueryRowContext(ctx, `SELECT id,name,type,cost_milli_ap,description,enabled FROM reward_catalog WHERE id=? AND enabled=1`, id).Scan(&r.ID, &r.Name, &r.Type, &r.CostMilliAP, &r.Description, &r.Enabled)
	if err == sql.ErrNoRows {
		return Redemption{}, fmt.Errorf("reward not found")
	}
	if err != nil {
		return Redemption{}, err
	}
	var focus, ledger int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(credited_focus_seconds),0) FROM motivation_daily`).Scan(&focus); err != nil {
		return Redemption{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(delta_milli_ap),0) FROM ap_ledger`).Scan(&ledger); err != nil {
		return Redemption{}, err
	}
	if apPerFocusHourMilli <= 0 {
		apPerFocusHourMilli = 1000
	}
	balance := focus*apPerFocusHourMilli/3600 + ledger
	if balance < r.CostMilliAP {
		return Redemption{}, fmt.Errorf("insufficient AP balance")
	}
	rid := fmt.Sprintf("redeem-%d", now.UnixNano())
	if _, err = tx.ExecContext(ctx, `INSERT INTO reward_redemptions(id,reward_id,reward_name,cost_milli_ap,redeemed_at) VALUES(?,?,?,?,?)`, rid, r.ID, r.Name, r.CostMilliAP, now); err != nil {
		return Redemption{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ap_ledger(id,source,reference_id,delta_milli_ap,created_at) VALUES(?,?,?,?,?)`, rid, "REWARD_REDEEM", rid, -r.CostMilliAP, now); err != nil {
		return Redemption{}, err
	}
	if err = tx.Commit(); err != nil {
		return Redemption{}, err
	}
	return Redemption{ID: rid, RewardID: r.ID, RewardName: r.Name, CostMilliAP: r.CostMilliAP, RedeemedAt: now}, nil
}
func (s *Storage) ListRedemptions(ctx context.Context, limit int) ([]Redemption, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,reward_id,reward_name,cost_milli_ap,redeemed_at FROM reward_redemptions ORDER BY redeemed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Redemption, 0)
	for rows.Next() {
		var r Redemption
		if err := rows.Scan(&r.ID, &r.RewardID, &r.RewardName, &r.CostMilliAP, &r.RedeemedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
