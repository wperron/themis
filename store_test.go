package themis

import (
	"context"
	_ "embed"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func TestStore_Claim(t *testing.T) {
	store, err := NewStore("file::memory:?cache=shared")
	assert.NoError(t, err)

	type args struct {
		player    string
		province  string
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
			},
			wantErr: false,
		},
		{
			name: "invalid name",
			args: args{
				player:    "foo",
				province:  "Italy",
				claimType: CLAIM_TYPE_TRADE, // Italy is a Region you silly goose
			},
			wantErr: true,
		},
		{
			name: "conflicts",
			args: args{
				player:    "bar",
				province:  "Genoa",
				claimType: CLAIM_TYPE_TRADE,
			},
			wantErr: true,
		},
		{
			name: "same player overlapp",
			args: args{
				player:    "foo", // 'foo' has a claim on Italy, which has overlapping provinces
				province:  "Genoa",
				claimType: CLAIM_TYPE_TRADE,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.Claim(context.TODO(), tt.args.player, tt.args.province, tt.args.claimType); (err != nil) != tt.wantErr {
				t.Errorf("Store.Claim() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAvailability(t *testing.T) {
	store, err := NewStore("file::memory:?cache=shared")
	assert.NoError(t, err)

	store.Claim(context.TODO(), "foo", "Genoa", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "foo", "Venice", CLAIM_TYPE_TRADE)
	store.Claim(context.TODO(), "foo", "English Channel", CLAIM_TYPE_TRADE)

	// There's a total of 80 distinct trade nodes, there should be 77 available
	// after the three claims above
	availability, err := store.ListAvailability(context.TODO(), CLAIM_TYPE_TRADE)
	assert.NoError(t, err)
	assert.Equal(t, 77, len(availability))

	store.Claim(context.TODO(), "foo", "France", CLAIM_TYPE_REGION)
	store.Claim(context.TODO(), "foo", "Italy", CLAIM_TYPE_REGION)

	// There's a total of 73 distinct regions, there should be 71 available
	// after the two claims above
	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_REGION)
	assert.NoError(t, err)
	assert.Equal(t, 71, len(availability))

	store.Claim(context.TODO(), "foo", "Normandy", CLAIM_TYPE_AREA)
	store.Claim(context.TODO(), "foo", "Champagne", CLAIM_TYPE_AREA)
	store.Claim(context.TODO(), "foo", "Lorraine", CLAIM_TYPE_AREA)
	store.Claim(context.TODO(), "foo", "Provence", CLAIM_TYPE_AREA)

	// There's a total of 823 distinct regions, there should be 819 available
	// after the four claims above
	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_AREA)
	assert.NoError(t, err)
	assert.Equal(t, 819, len(availability))

	// There is both a Trade Node and an Area called 'Valencia', while the trade
	// node is claimed, the area should show up in the availability list (even
	// though there are conflicting provinces)
	store.Claim(context.TODO(), "foo", "Valencia", CLAIM_TYPE_TRADE)
	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_AREA)
	assert.NoError(t, err)
	assert.Equal(t, 819, len(availability)) // availability for areas should be the same as before

	availability, err = store.ListAvailability(context.TODO(), CLAIM_TYPE_AREA, "bay")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(availability)) // availability for areas should be the same as before
}
