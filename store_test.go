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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.Claim(context.TODO(), tt.args.player, tt.args.province, tt.args.claimType); (err != nil) != tt.wantErr {
				t.Errorf("Store.Claim() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
