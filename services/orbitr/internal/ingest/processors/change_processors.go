package processors

import (
	"context"
	"io"

	"github.com/lantah/go/ingest"
	"github.com/lantah/go/support/errors"
)

type ChangeProcessor interface {
	ProcessChange(ctx context.Context, change ingest.Change) error
}

type LedgerTransactionProcessor interface {
	ProcessTransaction(ctx context.Context, transaction ingest.LedgerTransaction) error
}

type LedgerTransactionFilterer interface {
	FilterTransaction(ctx context.Context, transaction ingest.LedgerTransaction) (bool, error)
}

func StreamLedgerTransactions(
	ctx context.Context,
	txFilterer LedgerTransactionFilterer,
	filteredTxProcessor LedgerTransactionProcessor,
	txProcessor LedgerTransactionProcessor,
	reader *ingest.LedgerTransactionReader,
) error {
	for {
		tx, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "could not read transaction")
		}
		include, err := txFilterer.FilterTransaction(ctx, tx)
		if err != nil {
			return errors.Wrapf(
				err,
				"could not filter transaction %v",
				tx.Index,
			)
		}
		if !include {
			if err = filteredTxProcessor.ProcessTransaction(ctx, tx); err != nil {
				return errors.Wrapf(
					err,
					"could not process transaction %v",
					tx.Index,
				)
			}
			log.Debugf("Filters did not find match on transaction, dropping this tx with hash %v", tx.Result.TransactionHash.HexString())
			continue
		}

		if err = txProcessor.ProcessTransaction(ctx, tx); err != nil {
			return errors.Wrapf(
				err,
				"could not process transaction %v",
				tx.Index,
			)
		}
	}
}

func StreamChanges(
	ctx context.Context,
	changeProcessor ChangeProcessor,
	reader ingest.ChangeReader,
) error {
	for {
		change, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "could not read transaction")
		}

		if err = changeProcessor.ProcessChange(ctx, change); err != nil {
			return errors.Wrap(
				err,
				"could not process change",
			)
		}
	}
}
