package repository

import "time"

// SetSkippedUnsubscribed stores how many list recipients were left out at job
// start because they are on the unsubscribe stop list.
func (r *JobRepository) SetSkippedUnsubscribed(jobID string, n int) error {
	_, err := r.db.Exec("UPDATE send_jobs SET skipped_unsubscribed = ?, updated_at = ? WHERE id = ?",
		n, time.Now().UTC(), jobID)
	return err
}
