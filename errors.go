package themis

import (
	"errors"
	"fmt"
)

var ErrNoSuchClaim = errors.New("no such claim")

type ErrConflict struct {
	Conflicts []Conflict
}

func (ec ErrConflict) Error() string {
	return fmt.Sprintf("found %d conflicting provinces", len(ec.Conflicts))
}
