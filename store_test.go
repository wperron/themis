//nolint:errcheck
package themis

import (
	"context"
	_ "embed"
	"fmt"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

const TEST_CONN_STRING_PATTERN = "file:%s?mode=memory&cache=shared"

func TestStore_Claim(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "TestStore_Claim"))
	assert.NoError(t, err)

	type args struct {
		player    string
		province  string
		userId    string
		claimType ClaimType
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				player:    "foo",
				province:  "Italy",
				claimType: CLAIM_TYPE_REGION,
				userId:    "000000000000000001",
			},
			wantErr: false,
		},
		{
			name: "invalid name",
			args: args{
				player:    "foo",
				province:  "Italy",
				claimType: CLAIM_TYPE_TRADE, // Italy is a Region you silly goose
				userId:    "000000000000000001",
			},
			wantErr: true,
		},
		{
			name: "conflicts",
			args: args{
				player:    "bar",
				province:  "Genoa",
				claimType: CLAIM_TYPE_TRADE,
				userId:    "000000000000000002",
			},
			wantErr: true,
		},
		{
			name: "same player overlapp",
			args: args{
				player:    "foo", // 'foo' has a claim on Italy, which has overlapping provinces
				province:  "Genoa",
				claimType: CLAIM_TYPE_TRADE,
				userId:    "000000000000000001",
			},
			wantErr: false,
		},
		{
			name: "case sensitivity lower",
			args: args{
				player:    "foo",
				province:  "wien",
				claimType: CLAIM_TYPE_TRADE,
				userId:    "000000000000000001",
			},
			wantErr: false,
		},
		{
			name: "case sensitivity upper",
			args: args{
				player:    "foo",
				province:  "CONSTANTINOPLE",
				claimType: CLAIM_TYPE_TRADE,
				userId:    "000000000000000001",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Claim(context.TODO(), tt.args.userId, tt.args.player, tt.args.province, tt.args.claimType); (err != nil) != tt.wantErr {
				t.Errorf("Store.Claim() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAvailability(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "TestAvailability"))
	assert.NoError(t, err)

	store.Claim(context.TODO(), "000000000000000001", "foo", "Genoa", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Venice", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "000000000000000001", "foo", "English Channel", CLAIM_TYPE_TRADE)

	// There's a total of 80 distinct trade nodes, there should be 77 available
	// after the three claims above
	availability, err := store.ListAvailability(context.TODO(), CLAIM_TYPE_TRADE)
	assert.NoError(t, err)
	assert.Equal(t, 77, len(availability))

	store.Claim(context.TODO(), "000000000000000001", "foo", "France", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Italy", CLAIM_TYPE_REGION)

	// There's a total of 73 distinct regions, there should be 71 available
	// after the two claims above
	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_REGION)
	assert.NoError(t, err)
	assert.Equal(t, 71, len(availability))

	store.Claim(context.TODO(), "000000000000000001", "foo", "Normandy", CLAIM_TYPE_AREA)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Champagne", CLAIM_TYPE_AREA)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Lorraine", CLAIM_TYPE_AREA)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Provence", CLAIM_TYPE_AREA)

	// There's a total of 823 distinct regions, there should be 819 available
	// after the four claims above
	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_AREA)
	assert.NoError(t, err)
	assert.Equal(t, 819, len(availability))

	// There is both a Trade Node and an Area called 'Valencia', while the trade
	// node is claimed, the area should show up in the availability list (even
	// though there are conflicting provinces)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Valencia", CLAIM_TYPE_TRADE)
	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_AREA)
	assert.NoError(t, err)
	assert.Equal(t, 819, len(availability)) // availability for areas should be the same as before

	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_AREA, "bay")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(availability)) // availability for areas should be the same as before
}

func TestDeleteClaim(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "TestDeleteClaim"))
	assert.NoError(t, err)
	// make sure all claims are gone, this is due to how the in-memory database
	// with a shared cache interacts with other tests running in parallel
	_, err = store.db.ExecContext(context.TODO(), "DELETE FROM claims")
	assert.NoError(t, err)

	fooId, _ := store.Claim(context.TODO(), "000000000000000001", "foo", "Genoa", CLAIM_TYPE_TRADE)
	barId, _ := store.Claim(context.TODO(), "000000000000000002", "bar", "Balkans", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "000000000000000003", "baz", "English Channel", CLAIM_TYPE_TRADE)

	claims, err := store.ListClaims(context.TODO())
	assert.NoError(t, err)
	fmt.Print(claims)

	err = store.DeleteClaim(context.TODO(), fooId, "000000000000000001")
	assert.NoError(t, err)

	err = store.DeleteClaim(context.TODO(), barId, "000000000000000001")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoSuchClaim)
}

func TestDescribeClaim(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "TestDescribeClaim"))
	assert.NoError(t, err)

	id, err := store.Claim(context.TODO(), "000000000000000001", "foo", "Genoa", CLAIM_TYPE_TRADE)
	assert.NoError(t, err)

	detail, err := store.DescribeClaim(context.TODO(), id)
	assert.NoError(t, err)
	assert.Equal(t, "Genoa", detail.Name)
	assert.Contains(t, detail.Provinces, "Saluzzo")

	detail, err = store.DescribeClaim(context.TODO(), 9999)
	assert.ErrorIs(t, err, ErrNoSuchClaim)
}

func TestCountClaims(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "TestFlush"))
	assert.NoError(t, err)

	store.Claim(context.TODO(), "000000000000000001", "foo", "Genoa", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Valencia", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Italy", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Iberia", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Ragusa", CLAIM_TYPE_TRADE)

	total, uniquePlayers, err := store.CountClaims(context.TODO())
	assert.NoError(t, err)
	assert.Condition(t, func() bool { return total > 0 })
	assert.Condition(t, func() bool { return uniquePlayers > 0 })
}

func TestFlush(t *testing.T) {
	store, err := NewStore(fmt.Sprintf(TEST_CONN_STRING_PATTERN, "TestFlush"))
	assert.NoError(t, err)

	store.Claim(context.TODO(), "000000000000000001", "foo", "Genoa", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Valencia", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Italy", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Iberia", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "000000000000000001", "foo", "Ragusa", CLAIM_TYPE_TRADE)

	assert.NoError(t, store.Flush(context.TODO()))
	claims, err := store.ListClaims(context.TODO())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(claims))
}
