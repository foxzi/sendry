package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Segment narrows a recipient list for one launch: only the recipients that
// match the conditions get the email. It is stored with the send job as JSON.
type Segment struct {
	Match      string             `json:"match"` // all, any
	Conditions []SegmentCondition `json:"conditions"`
}

// SegmentCondition is one condition of a segment. Key is the variable name
// (field "var" only); Value is a number of days, a date (YYYY-MM-DD, UTC), a
// tag or a text, depending on the field and the operator.
type SegmentCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// Segment condition fields
const (
	SegmentFieldAdded    = "added"    // when the address was added to the list
	SegmentFieldOpened   = "opened"   // opened campaign emails
	SegmentFieldClicked  = "clicked"  // clicked links in campaign emails
	SegmentFieldReceived = "received" // was sent campaign emails
	SegmentFieldTag      = "tag"
	SegmentFieldVar      = "var"
	SegmentFieldEmail    = "email"
)

// Segment condition operators
const (
	SegmentOpLastDays    = "last_days"
	SegmentOpNotLastDays = "not_last_days"
	SegmentOpAfter       = "after"
	SegmentOpBefore      = "before"
	SegmentOpNever       = "never"
	SegmentOpHas         = "has"
	SegmentOpNotHas      = "not_has"
	SegmentOpEq          = "eq"
	SegmentOpNe          = "ne"
	SegmentOpContains    = "contains"
	SegmentOpNotContains = "not_contains"
	SegmentOpGt          = "gt"
	SegmentOpLt          = "lt"
	SegmentOpSet         = "set"
	SegmentOpNotSet      = "not_set"
)

// SegmentDateLayout is the format of the date values of a condition
const SegmentDateLayout = "2006-01-02"

const (
	maxSegmentConditions = 20
	maxSegmentValueLen   = 200
	maxSegmentDays       = 36500
)

var segmentActivityOps = []string{SegmentOpLastDays, SegmentOpNotLastDays, SegmentOpAfter, SegmentOpBefore, SegmentOpNever}

// segmentOps lists the operators each field accepts
var segmentOps = map[string][]string{
	SegmentFieldAdded:    {SegmentOpLastDays, SegmentOpAfter, SegmentOpBefore},
	SegmentFieldOpened:   segmentActivityOps,
	SegmentFieldClicked:  segmentActivityOps,
	SegmentFieldReceived: segmentActivityOps,
	SegmentFieldTag:      {SegmentOpHas, SegmentOpNotHas},
	SegmentFieldVar:      {SegmentOpEq, SegmentOpNe, SegmentOpContains, SegmentOpGt, SegmentOpLt, SegmentOpSet, SegmentOpNotSet},
	SegmentFieldEmail:    {SegmentOpContains, SegmentOpNotContains},
}

// ParseSegment reads the JSON of a segment and validates it. An empty string
// (or JSON null) is "no segment": nil without an error.
func ParseSegment(raw string) (*Segment, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var s Segment
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, errors.New("not a valid JSON")
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Validate checks the segment and normalizes it (trimmed values, match "all"
// by default).
func (s *Segment) Validate() error {
	if s.Match == "" {
		s.Match = "all"
	}
	if s.Match != "all" && s.Match != "any" {
		return fmt.Errorf("unknown match mode %q", s.Match)
	}
	if len(s.Conditions) == 0 {
		return errors.New("no conditions")
	}
	if len(s.Conditions) > maxSegmentConditions {
		return fmt.Errorf("at most %d conditions", maxSegmentConditions)
	}
	for i := range s.Conditions {
		if err := s.Conditions[i].validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i+1, err)
		}
	}
	return nil
}

func (c *SegmentCondition) validate() error {
	ops, ok := segmentOps[c.Field]
	if !ok {
		return fmt.Errorf("unknown field %q", c.Field)
	}
	known := false
	for _, op := range ops {
		known = known || op == c.Op
	}
	if !known {
		return fmt.Errorf("unknown operator %q for %q", c.Op, c.Field)
	}

	c.Key = strings.TrimSpace(c.Key)
	c.Value = strings.TrimSpace(c.Value)
	if utf8.RuneCountInString(c.Key) > maxSegmentValueLen || utf8.RuneCountInString(c.Value) > maxSegmentValueLen {
		return errors.New("value is too long")
	}
	if c.Field == SegmentFieldVar {
		if c.Key == "" {
			return errors.New("variable name is required")
		}
		// the name goes into a JSON path
		if strings.ContainsAny(c.Key, `"\`) {
			return errors.New("variable name must not contain quotes or backslashes")
		}
	} else {
		c.Key = ""
	}

	switch c.Op {
	case SegmentOpNever, SegmentOpSet, SegmentOpNotSet:
		c.Value = ""
	case SegmentOpLastDays, SegmentOpNotLastDays:
		if _, err := c.Days(); err != nil {
			return err
		}
	case SegmentOpAfter, SegmentOpBefore:
		if _, err := c.Date(); err != nil {
			return err
		}
	case SegmentOpGt, SegmentOpLt:
		if _, err := c.Number(); err != nil {
			return err
		}
	default:
		if c.Value == "" {
			return errors.New("value is required")
		}
	}
	return nil
}

// Days is the value of a "last N days" condition
func (c SegmentCondition) Days() (int, error) {
	n, err := strconv.Atoi(c.Value)
	if err != nil || n < 1 || n > maxSegmentDays {
		return 0, fmt.Errorf("number of days must be between 1 and %d", maxSegmentDays)
	}
	return n, nil
}

// Date is the value of a date condition: the start of that day in UTC
func (c SegmentCondition) Date() (time.Time, error) {
	t, err := time.Parse(SegmentDateLayout, c.Value)
	if err != nil {
		return time.Time{}, errors.New("date must be YYYY-MM-DD")
	}
	return t, nil
}

// Number is the value of a numeric comparison; a decimal comma is accepted
func (c SegmentCondition) Number() (float64, error) {
	n, err := strconv.ParseFloat(strings.Replace(c.Value, ",", ".", 1), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, errors.New("value must be a number")
	}
	return n, nil
}

// JSON returns the segment as stored with a job; "" for no segment
func (s *Segment) JSON() string {
	if s == nil {
		return ""
	}
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b)
}
