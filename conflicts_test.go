package themis

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_FindConflicts(t *testing.T) {
	store, err := NewStore(TEST_CONN_STRING)
	assert.NoError(t, err)

	id, err := store.Claim(context.TODO(), "000000000000000001", "foo", "Bordeaux", CLAIM_TYPE_TRADE)
	assert.NoError(t, err)

	type args struct {
		ctx       context.Context
		userId    string
		name      string
		claimType ClaimType
	}
	tests := []struct {
		name    string
		args    args
		want    []Conflict
		wantErr bool
	}{
		{
			name: "same-player",
			args: args{
				context.TODO(),
				"000000000000000001",
				"France",
				CLAIM_TYPE_REGION,
			},
			want:    []Conflict{},
			wantErr: false,
		},
		{
			name: "overlapping",
			args: args{
				context.TODO(),
				"000000000000000002",
				"Iberia",
				CLAIM_TYPE_REGION,
			},
			want: []Conflict{
				{Province: "Navarra", Player: "foo", ClaimType: "trade", Claim: "Bordeaux", ClaimID: id},
				{Province: "Rioja", Player: "foo", ClaimType: "trade", Claim: "Bordeaux", ClaimID: id},
				{Province: "Vizcaya", Player: "foo", ClaimType: "trade", Claim: "Bordeaux", ClaimID: id},
			},
			wantErr: false,
		},
		{
			name: "no-overlap",
			args: args{
				context.TODO(),
				"000000000000000002",
				"Scandinavia",
				CLAIM_TYPE_REGION,
			},
			want:    []Conflict{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.FindConflicts(tt.args.ctx, tt.args.userId, tt.args.name, tt.args.claimType)
			if (err != nil) != tt.wantErr {
				t.Errorf("Store.FindConflicts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Store.FindConflicts() = %v, want %v", got, tt.want)
			}
		})
	}
}
