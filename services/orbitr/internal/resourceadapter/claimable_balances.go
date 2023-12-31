package resourceadapter

import (
	"context"
	"fmt"

	"github.com/lantah/go/amount"
	protocol "github.com/lantah/go/protocols/orbitr"
	orbitrContext "github.com/lantah/go/services/orbitr/internal/context"
	"github.com/lantah/go/services/orbitr/internal/db2/history"
	"github.com/lantah/go/support/render/hal"
	"github.com/lantah/go/xdr"
)

// PopulateClaimableBalance fills out the resource's fields
func PopulateClaimableBalance(
	ctx context.Context,
	dest *protocol.ClaimableBalance,
	claimableBalance history.ClaimableBalance,
	ledger *history.Ledger,
) error {
	dest.BalanceID = claimableBalance.BalanceID
	dest.Asset = claimableBalance.Asset.StringCanonical()
	dest.Amount = amount.StringFromInt64(int64(claimableBalance.Amount))
	if claimableBalance.Sponsor.Valid {
		dest.Sponsor = claimableBalance.Sponsor.String
	}
	dest.LastModifiedLedger = claimableBalance.LastModifiedLedger
	dest.Claimants = make([]protocol.Claimant, len(claimableBalance.Claimants))
	for i, c := range claimableBalance.Claimants {
		dest.Claimants[i].Destination = c.Destination
		dest.Claimants[i].Predicate = c.Predicate
	}

	if ledger != nil {
		dest.LastModifiedTime = &ledger.ClosedAt
	}

	if xdr.ClaimableBalanceFlags(claimableBalance.Flags).IsClawbackEnabled() {
		dest.Flags.ClawbackEnabled = xdr.ClaimableBalanceFlags(claimableBalance.Flags).IsClawbackEnabled()
	}

	lb := hal.LinkBuilder{Base: orbitrContext.BaseURL(ctx)}
	self := fmt.Sprintf("/claimable_balances/%s", dest.BalanceID)
	dest.Links.Self = lb.Link(self)
	dest.PT = fmt.Sprintf("%d-%s", claimableBalance.LastModifiedLedger, dest.BalanceID)
	dest.Links.Transactions = lb.PagedLink(self, "transactions")
	dest.Links.Operations = lb.PagedLink(self, "operations")
	return nil
}
