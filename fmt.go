package themis

import (
	"database/sql"
	"fmt"
	"strings"
)

func FormatRows(rows *sql.Rows) (string, error) {
	sb := strings.Builder{}

	cols, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("failed to get rows columns: %w", err)
	}

	c := make([]string, len(cols))
	for i := range c {
		c[i] = " %-*s "
	}
	pattern := fmt.Sprintf("|%s|\n", strings.Join(c, "|"))

	lengths := make([]int, len(cols))
	for i := range lengths {
		lengths[i] = len(cols[i])
	}

	scanned := make([][]any, 0)
	for rows.Next() {
		row := make([]interface{}, len(cols))
		for i := range row {
			row[i] = new(sql.NullString)
		}
		if err := rows.Scan(row...); err != nil {
			return "", fmt.Errorf("failed to scan next row: %w", err)
		}

		scanned = append(scanned, row) // keep track of row for later
		for i, a := range row {
			s := a.(*sql.NullString)
			if len(s.String) > lengths[i] {
				lengths[i] = len(s.String)
			}
		}
	}

	// Write column names
	curr := make([]any, 0, 2*len(cols))
	for i := range lengths {
		curr = append(curr, lengths[i], cols[i])
	}
	sb.WriteString(fmt.Sprintf(pattern, curr...))

	// Write header separator row
	curr = curr[:0] // empty slice but preserve capacity
	for i := range lengths {
		curr = append(curr, lengths[i], strings.Repeat("-", lengths[i]))
	}
	sb.WriteString(fmt.Sprintf(pattern, curr...))

	// iterate rows and write each one
	for _, r := range scanned {
		curr = curr[:0] // empty slice but preserve capacity
		for i := range lengths {
			s := r[i].(*sql.NullString)
			curr = append(curr, lengths[i], *s)
		}
		sb.WriteString(fmt.Sprintf(pattern, curr...))
	}

	return sb.String(), nil
}
