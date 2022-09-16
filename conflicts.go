package themis

import (
	"context"
	"fmt"
)

type Conflict struct {
	Province  string
	Player    string
	ClaimType ClaimType
	Claim     string
	ClaimID   int
}

func (c Conflict) String() string {
	return fmt.Sprintf("%s owned by #%d %s %s (%s)", c.Province, c.ClaimID, c.ClaimType, c.Claim, c.Player)
}

const conflictQuery string = `SELECT name, player, claim_type, val, id FROM (
    SELECT provinces.name, claims.player, claims.claim_type, claims.val, claims.id
        FROM claims
        LEFT JOIN provinces ON claims.val = provinces.trade_node
        WHERE claims.claim_type = 'trade' AND claims.userid IS NOT ?
        AND provinces.%[1]s = ?
    UNION
        SELECT provinces.name, claims.player, claims.claim_type, claims.val, claims.id
        FROM claims
        LEFT JOIN provinces ON claims.val = provinces.region
        WHERE claims.claim_type = 'region' AND claims.userid IS NOT ?
        AND provinces.%[1]s = ?
    UNION
        SELECT provinces.name, claims.player, claims.claim_type, claims.val, claims.id
        FROM claims
        LEFT JOIN provinces ON claims.val = provinces.area
        WHERE claims.claim_type = 'area' AND claims.userid IS NOT ?
        AND provinces.%[1]s = ?
);`

func (s *Store) FindConflicts(ctx context.Context, userId, name string, claimType ClaimType) ([]Conflict, error) {
	stmt, err := s.db.PrepareContext(ctx, fmt.Sprintf(conflictQuery, claimTypeToColumn[claimType]))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare conflicts query: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, userId, name, userId, name, userId, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get conflicting provinces: %w", err)
	}

	conflicts := make([]Conflict, 0)
	for rows.Next() {
		var (
			province   string
			player     string
			sClaimType string
			claimName  string
			claimId    int
		)
		err = rows.Scan(&province, &player, &sClaimType, &claimName, &claimId)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		ct, err := ClaimTypeFromString(sClaimType)
		if err != nil {
			// In case of an error parsing the claim type, simply default to
			// whatever the database sends; this is a read-only function, the
			// input validation is assumed to have already been done at insert.
			ct = ClaimType(sClaimType)
		}
		conflicts = append(conflicts, Conflict{
			Province:  province,
			Player:    player,
			ClaimType: ct,
			Claim:     claimName,
			ClaimID:   claimId,
		})
	}

	return conflicts, nil
}
