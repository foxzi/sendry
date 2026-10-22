package repository

import (
	"errors"

	"github.com/mattn/go-sqlite3"
)

// ErrInUse is returned when a row cannot be deleted because other rows still
// reference it (a foreign key), e.g. a recipient list that campaign jobs were
// sent to.
var ErrInUse = errors.New("still in use")

// inUse maps a foreign key violation to ErrInUse.
func inUse(err error) error {
	var se sqlite3.Error
	if errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintForeignKey {
		return ErrInUse
	}
	return err
}
