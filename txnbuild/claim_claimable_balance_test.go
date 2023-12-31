package txnbuild

import (
	"testing"
)

func TestClaimClaimableBalanceRoundTrip(t *testing.T) {
	claimClaimableBalance := &ClaimClaimableBalance{
		SourceAccount: "GB7BDSZU2Y27LYNLALKKALB52WS2IZWYBDGY6EQBLEED3TJOCVMZRH7H",
		BalanceID:     "00000000929b20b72e5890ab51c24f1cc46fa01c4f318d8d33367d24dd614cfdf5491072",
	}

	testOperationsMarshalingRoundtrip(t, []Operation{claimClaimableBalance}, false)

	// with muxed accounts
	claimClaimableBalance = &ClaimClaimableBalance{
		SourceAccount: "MA7QYNF7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74P7UJVAAAAAAAAAAAAAJLK",
		BalanceID:     "00000000929b20b72e5890ab51c24f1cc46fa01c4f318d8d33367d24dd614cfdf5491072",
	}

	testOperationsMarshalingRoundtrip(t, []Operation{claimClaimableBalance}, true)
}
