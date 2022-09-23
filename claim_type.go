package themis

import "fmt"

type ClaimType string

func ClaimTypeFromString(s string) (ClaimType, error) {
	switch s {
	case CLAIM_TYPE_AREA:
		return CLAIM_TYPE_AREA, nil
	case CLAIM_TYPE_REGION:
		return CLAIM_TYPE_REGION, nil
	case CLAIM_TYPE_TRADE:
		return CLAIM_TYPE_TRADE, nil
	}
	return "", fmt.Errorf("no claim type matching '%s'", s)
}

func (ct ClaimType) String() string {
	switch ct {
	case CLAIM_TYPE_AREA:
		return "Area"
	case CLAIM_TYPE_REGION:
		return "Region"
	case CLAIM_TYPE_TRADE:
		return "Trade Node"
	}
	return ""
}

const (
	CLAIM_TYPE_AREA   = "area"
	CLAIM_TYPE_REGION = "region"
	CLAIM_TYPE_TRADE  = "trade"
)

var claimTypeToColumn = map[ClaimType]string{
	CLAIM_TYPE_AREA:   "area",
	CLAIM_TYPE_REGION: "region",
	CLAIM_TYPE_TRADE:  "trade_node",
}

type Claim struct {
	ID     int
	Player string
	Name   string
	Type   ClaimType
}

func (c Claim) String() string {
	return fmt.Sprintf("id=%d player=%s claim_type=%s name=%s", c.ID, c.Player, c.Type, c.Name)
}
