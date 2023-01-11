package themis

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Uptime returns the time elapsed since the start of the current process ID.
func Uptime() (time.Duration, error) {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("failed to read uptime from OS: %w", err)
	}

	i := bytes.IndexRune(raw, ' ')

	up, err := strconv.ParseFloat(string(raw[:i]), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse uptime from OS: %w", err)
	}

	return time.Duration(int(up*1000) * int(time.Millisecond)), nil
}
