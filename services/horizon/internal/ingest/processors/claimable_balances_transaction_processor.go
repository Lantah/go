package processors

import (
	"context"

	"github.com/lantah/go/ingest"
	"github.com/lantah/go/services/orbitr/internal/db2/history"
	set "github.com/lantah/go/support/collections/set"
	"github.com/lantah/go/support/errors"
	"github.com/lantah/go/toid"
	"github.com/lantah/go/xdr"
)

type claimableBalance struct {
	internalID     int64 // Bigint auto-generated by postgres
	transactionSet set.Set[int64]
	operationSet   set.Set[int64]
}

func (b *claimableBalance) addTransactionID(id int64) {
	if b.transactionSet == nil {
		b.transactionSet = set.Set[int64]{}
	}
	b.transactionSet.Add(id)
}

func (b *claimableBalance) addOperationID(id int64) {
	if b.operationSet == nil {
		b.operationSet = set.Set[int64]{}
	}
	b.operationSet.Add(id)
}

type ClaimableBalancesTransactionProcessor struct {
	sequence            uint32
	claimableBalanceSet map[string]claimableBalance
	qClaimableBalances  history.QHistoryClaimableBalances
}

func NewClaimableBalancesTransactionProcessor(Q history.QHistoryClaimableBalances, sequence uint32) *ClaimableBalancesTransactionProcessor {
	return &ClaimableBalancesTransactionProcessor{
		qClaimableBalances:  Q,
		sequence:            sequence,
		claimableBalanceSet: map[string]claimableBalance{},
	}
}

func (p *ClaimableBalancesTransactionProcessor) ProcessTransaction(ctx context.Context, transaction ingest.LedgerTransaction) error {
	err := p.addTransactionClaimableBalances(p.claimableBalanceSet, p.sequence, transaction)
	if err != nil {
		return err
	}

	err = p.addOperationClaimableBalances(p.claimableBalanceSet, p.sequence, transaction)
	if err != nil {
		return err
	}

	return nil
}

func (p *ClaimableBalancesTransactionProcessor) addTransactionClaimableBalances(cbSet map[string]claimableBalance, sequence uint32, transaction ingest.LedgerTransaction) error {
	transactionID := toid.New(int32(sequence), int32(transaction.Index), 0).ToInt64()
	transactionClaimableBalances, err := claimableBalancesForTransaction(
		sequence,
		transaction,
	)
	if err != nil {
		return errors.Wrap(err, "Could not determine claimable balances for transaction")
	}

	for _, cb := range transactionClaimableBalances {
		entry := cbSet[cb]
		entry.addTransactionID(transactionID)
		cbSet[cb] = entry
	}

	return nil
}

func claimableBalancesForTransaction(
	sequence uint32,
	transaction ingest.LedgerTransaction,
) ([]string, error) {
	changes, err := transaction.GetChanges()
	if err != nil {
		return nil, err
	}
	cbs, err := claimableBalancesForChanges(changes)
	if err != nil {
		return nil, errors.Wrapf(err, "reading transaction %v claimable balances", transaction.Index)
	}
	return dedupeClaimableBalances(cbs)
}

func dedupeClaimableBalances(in []string) (out []string, err error) {
	set := set.Set[string]{}
	for _, id := range in {
		set.Add(id)
	}

	for id := range set {
		out = append(out, id)
	}
	return
}

func claimableBalancesForChanges(
	changes []ingest.Change,
) ([]string, error) {
	var cbs []string

	for _, c := range changes {
		if c.Type != xdr.LedgerEntryTypeClaimableBalance {
			continue
		}

		if c.Pre == nil && c.Post == nil {
			return nil, errors.New("Invalid io.Change: change.Pre == nil && change.Post == nil")
		}

		var claimableBalanceID xdr.ClaimableBalanceId
		if c.Pre != nil {
			claimableBalanceID = c.Pre.Data.MustClaimableBalance().BalanceId
		}
		if c.Post != nil {
			claimableBalanceID = c.Post.Data.MustClaimableBalance().BalanceId
		}
		id, err := xdr.MarshalHex(claimableBalanceID)
		if err != nil {
			return nil, err
		}
		cbs = append(cbs, id)
	}

	return cbs, nil
}

func (p *ClaimableBalancesTransactionProcessor) addOperationClaimableBalances(cbSet map[string]claimableBalance, sequence uint32, transaction ingest.LedgerTransaction) error {
	claimableBalances, err := claimableBalancesForOperations(transaction, sequence)
	if err != nil {
		return errors.Wrap(err, "could not determine operation claimable balances")
	}

	for operationID, cbs := range claimableBalances {
		for _, cb := range cbs {
			entry := cbSet[cb]
			entry.addOperationID(operationID)
			cbSet[cb] = entry
		}
	}

	return nil
}

func claimableBalancesForOperations(transaction ingest.LedgerTransaction, sequence uint32) (map[int64][]string, error) {
	cbs := map[int64][]string{}

	for opi, op := range transaction.Envelope.Operations() {
		operation := transactionOperationWrapper{
			index:          uint32(opi),
			transaction:    transaction,
			operation:      op,
			ledgerSequence: sequence,
		}

		changes, err := transaction.GetOperationChanges(uint32(opi))
		if err != nil {
			return cbs, err
		}
		c, err := claimableBalancesForChanges(changes)
		if err != nil {
			return cbs, errors.Wrapf(err, "reading operation %v claimable balances", operation.ID())
		}
		cbs[operation.ID()] = c
	}

	return cbs, nil
}

func (p *ClaimableBalancesTransactionProcessor) Commit(ctx context.Context) error {
	if len(p.claimableBalanceSet) > 0 {
		if err := p.loadClaimableBalanceIDs(ctx, p.claimableBalanceSet); err != nil {
			return err
		}

		if err := p.insertDBTransactionClaimableBalances(ctx, p.claimableBalanceSet); err != nil {
			return err
		}

		if err := p.insertDBOperationsClaimableBalances(ctx, p.claimableBalanceSet); err != nil {
			return err
		}
	}

	return nil
}

func (p *ClaimableBalancesTransactionProcessor) loadClaimableBalanceIDs(ctx context.Context, claimableBalanceSet map[string]claimableBalance) error {
	ids := make([]string, 0, len(claimableBalanceSet))
	for id := range claimableBalanceSet {
		ids = append(ids, id)
	}

	toInternalID, err := p.qClaimableBalances.CreateHistoryClaimableBalances(ctx, ids, maxBatchSize)
	if err != nil {
		return errors.Wrap(err, "Could not create claimable balance ids")
	}

	for _, id := range ids {
		internalID, ok := toInternalID[id]
		if !ok {
			// TODO: Figure out the right way to convert the id to a string here. %v will be nonsense.
			return errors.Errorf("no internal id found for claimable balance %v", id)
		}

		cb := claimableBalanceSet[id]
		cb.internalID = internalID
		claimableBalanceSet[id] = cb
	}

	return nil
}

func (p ClaimableBalancesTransactionProcessor) insertDBTransactionClaimableBalances(ctx context.Context, claimableBalanceSet map[string]claimableBalance) error {
	batch := p.qClaimableBalances.NewTransactionClaimableBalanceBatchInsertBuilder(maxBatchSize)

	for _, entry := range claimableBalanceSet {
		for transactionID := range entry.transactionSet {
			if err := batch.Add(ctx, transactionID, entry.internalID); err != nil {
				return errors.Wrap(err, "could not insert transaction claimable balance in db")
			}
		}
	}

	if err := batch.Exec(ctx); err != nil {
		return errors.Wrap(err, "could not flush transaction claimable balances to db")
	}
	return nil
}

func (p ClaimableBalancesTransactionProcessor) insertDBOperationsClaimableBalances(ctx context.Context, claimableBalanceSet map[string]claimableBalance) error {
	batch := p.qClaimableBalances.NewOperationClaimableBalanceBatchInsertBuilder(maxBatchSize)

	for _, entry := range claimableBalanceSet {
		for operationID := range entry.operationSet {
			if err := batch.Add(ctx, operationID, entry.internalID); err != nil {
				return errors.Wrap(err, "could not insert operation claimable balance in db")
			}
		}
	}

	if err := batch.Exec(ctx); err != nil {
		return errors.Wrap(err, "could not flush operation claimable balances to db")
	}
	return nil
}
