package themis

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatRows(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "format-rows"))
	assert.NoError(t, err)

	rows, err := store.db.Query("SELECT provinces.name, provinces.region, provinces.area, provinces.trade_node FROM provinces WHERE area = 'Gascony'")
	assert.NoError(t, err)

	fmtd, err := FormatRows(rows)
	assert.NoError(t, err)
	assert.Equal(t, `| name     | region | area    | trade_node |
| -------- | ------ | ------- | ---------- |
| Labourd  | France | Gascony | Bordeaux   |
| Armagnac | France | Gascony | Bordeaux   |
| Béarn    | France | Gascony | Bordeaux   |
| Foix     | France | Gascony | Bordeaux   |
`, fmtd)
}

func TestFormatRowsAggregated(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "format-rows"))
	assert.NoError(t, err)

	rows, err := store.db.Query("SELECT count(1) as total, trade_node from provinces where region = 'France' group by trade_node")
	assert.NoError(t, err)

	fmtd, err := FormatRows(rows)
	assert.NoError(t, err)
	assert.Equal(t, `| total | trade_node      |
| ----- | --------------- |
| 25    | Bordeaux        |
| 24    | Champagne       |
| 8     | English Channel |
| 4     | Genoa           |
| 5     | Valencia        |
`, fmtd)
}

func TestFormatRowsInvalidQuery(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "format-rows"))
	assert.NoError(t, err)

	_, err = store.db.Query("SELECT count(name), distinct(trade_node) from provinces where region = 'France'")
	assert.Error(t, err)
}
