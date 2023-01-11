package themis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUptime(t *testing.T) {
	uptime, err := Uptime()
	assert.NoError(t, err)
	assert.Greater(t, uptime, 100*time.Millisecond)
}
