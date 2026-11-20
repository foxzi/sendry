package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

// Timestamps are stored in several text forms (Go time values, SQLite
// CURRENT_TIMESTAMP, plain dates), so segment conditions compare them through
// datetime(), which brings all of them to UTC.
const segmentTimeLayout = "2006-01-02 15:04:05"

// segmentActivityColumns maps an activity field to the send_job_items column
// that is set when it happened.
var segmentActivityColumns = map[string]string{
	models.SegmentFieldOpened:   "opened_at",
	models.SegmentFieldClicked:  "clicked_at",
	models.SegmentFieldReceived: "sent_at",
}

// segmentWhere returns the SQL of a segment for a query over "recipients r":
// " AND (...)" with its arguments, or "" without a segment. now is the moment
// "last N days" is counted from. The segment must be validated.
func segmentWhere(seg *models.Segment, now time.Time) (string, []any, error) {
	if seg == nil || len(seg.Conditions) == 0 {
		return "", nil, nil
	}
	parts := make([]string, 0, len(seg.Conditions))
	var args []any
	for _, c := range seg.Conditions {
		q, a, err := segmentCondition(c, now)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, "("+q+")")
		args = append(args, a...)
	}
	glue := " AND "
	if seg.Match == "any" {
		glue = " OR "
	}
	return " AND (" + strings.Join(parts, glue) + ")", args, nil
}

func segmentCondition(c models.SegmentCondition, now time.Time) (string, []any, error) {
	switch c.Field {
	case models.SegmentFieldAdded:
		cmp, at, err := segmentTimeBound(c, now)
		if err != nil {
			return "", nil, err
		}
		return "datetime(r.created_at) " + cmp + " datetime(?)", []any{at}, nil

	case models.SegmentFieldOpened, models.SegmentFieldClicked, models.SegmentFieldReceived:
		// activity of the address in every list, like the recipient card
		col := segmentActivityColumns[c.Field]
		exists := `EXISTS (SELECT 1 FROM send_job_items i JOIN recipients r2 ON r2.id = i.recipient_id
			WHERE lower(r2.email) = lower(r.email) AND i.` + col + ` IS NOT NULL`
		if c.Op == models.SegmentOpNever {
			return "NOT " + exists + ")", nil, nil
		}
		cmp, at, err := segmentTimeBound(c, now)
		if err != nil {
			return "", nil, err
		}
		q := exists + " AND datetime(i." + col + ") " + cmp + " datetime(?))"
		if c.Op == models.SegmentOpNotLastDays {
			q = "NOT " + q
		}
		return q, []any{at}, nil

	case models.SegmentFieldTag:
		q := `EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(r.tags) THEN r.tags ELSE '[]' END) WHERE value = ?)`
		if c.Op == models.SegmentOpNotHas {
			q = "NOT " + q
		}
		return q, []any{c.Value}, nil

	case models.SegmentFieldVar:
		// the value of the variable as text, as the recipient card shows it;
		// '' when it is not set
		val := `seg_var(COALESCE(r.variables, ''), ?)`
		key := c.Key
		switch c.Op {
		case models.SegmentOpEq:
			return "ulower(" + val + ") = ulower(?)", []any{key, c.Value}, nil
		case models.SegmentOpNe:
			return "ulower(" + val + ") != ulower(?)", []any{key, c.Value}, nil
		case models.SegmentOpContains:
			return "ulower(" + val + `) LIKE ? ESCAPE '\'`, []any{key, searchPattern(c.Value)}, nil
		case models.SegmentOpSet:
			return val + " != ''", []any{key}, nil
		case models.SegmentOpNotSet:
			return val + " = ''", []any{key}, nil
		case models.SegmentOpGt, models.SegmentOpLt:
			n, err := c.Number()
			if err != nil {
				return "", nil, err
			}
			cmp := ">"
			if c.Op == models.SegmentOpLt {
				cmp = "<"
			}
			// a value that is not a number (unset, "n/a") is NULL: neither
			// greater nor less than anything
			return "seg_num(" + val + ") " + cmp + " ?", []any{key, n}, nil
		}

	case models.SegmentFieldEmail:
		q := `lower(r.email) LIKE ? ESCAPE '\'`
		if c.Op == models.SegmentOpNotContains {
			q = "NOT " + q
		}
		return q, []any{searchPattern(c.Value)}, nil
	}
	return "", nil, fmt.Errorf("unknown segment condition %s/%s", c.Field, c.Op)
}

// segmentTimeBound returns the comparison and the UTC moment of a date
// condition: "last N days" and "after" are ">= moment", "before" is "< moment".
func segmentTimeBound(c models.SegmentCondition, now time.Time) (string, string, error) {
	switch c.Op {
	case models.SegmentOpLastDays, models.SegmentOpNotLastDays:
		days, err := c.Days()
		if err != nil {
			return "", "", err
		}
		return ">=", now.UTC().AddDate(0, 0, -days).Format(segmentTimeLayout), nil
	case models.SegmentOpAfter, models.SegmentOpBefore:
		day, err := c.Date()
		if err != nil {
			return "", "", err
		}
		cmp := ">="
		if c.Op == models.SegmentOpBefore {
			cmp = "<"
		}
		return cmp, day.Format(segmentTimeLayout), nil
	}
	return "", "", fmt.Errorf("unknown segment operator %s", c.Op)
}

// StreamSegment is StreamRecipients narrowed to the recipients that match the
// segment (nil = the whole list). Caller must Close() the rows.
func (r *RecipientRepository) StreamSegment(listID string, seg *models.Segment) (*sql.Rows, error) {
	where, args, err := segmentWhere(seg, time.Now())
	if err != nil {
		return nil, err
	}
	return r.db.Query(`
		SELECT `+recipientColumns+`
		FROM recipients r WHERE r.list_id = ?`+where+`
		ORDER BY r.created_at DESC`, append([]any{listID}, args...)...)
}

// CountSegment returns how many active recipients of the list match the
// segment: the number a launch with it would send to.
func (r *RecipientRepository) CountSegment(listID string, seg *models.Segment) (int, error) {
	where, args, err := segmentWhere(seg, time.Now())
	if err != nil {
		return 0, err
	}
	var n int
	err = r.db.QueryRow(`SELECT COUNT(*) FROM recipients r WHERE r.list_id = ? AND r.status = 'active'`+where,
		append([]any{listID}, args...)...).Scan(&n)
	return n, err
}

// GetVariableKeys returns the names of the variables the recipients of a list
// have, sorted.
func (r *RecipientRepository) GetVariableKeys(listID string) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT j.key
		FROM recipients r, json_each(CASE WHEN json_valid(r.variables) THEN r.variables ELSE '{}' END) j
		WHERE r.list_id = ? AND json_type(CASE WHEN json_valid(r.variables) THEN r.variables ELSE '{}' END) = 'object'
		ORDER BY j.key`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
