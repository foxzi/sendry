package server

import (
	"context"
	"database/sql"
	"time"
)

// sessionCleanupInterval is how often expired sessions are purged
const sessionCleanupInterval = time.Hour

// purgeExpiredSessions deletes sessions that expired before now.
// expires_at is written by the sqlite driver from a time.Time, so it is
// compared with a time.Time parameter formatted the same way.
func purgeExpiredSessions(db *sql.DB, now time.Time) (int64, error) {
	res, err := db.Exec("DELETE FROM sessions WHERE expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// sessionCleanupLoop purges expired sessions at startup and then every
// sessionCleanupInterval until ctx is done.
func (s *Server) sessionCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for {
		if n, err := purgeExpiredSessions(s.db.DB, time.Now()); err != nil {
			s.logger.Error("failed to purge expired sessions", "error", err)
		} else if n > 0 {
			s.logger.Info("purged expired sessions", "count", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
