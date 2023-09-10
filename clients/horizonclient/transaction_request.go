package orbitrclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	hProtocol "github.com/lantah/go/protocols/orbitr"
	"github.com/lantah/go/support/errors"
)

// BuildURL creates the endpoint to be queried based on the data in the TransactionRequest struct.
// If no data is set, it defaults to the build the URL for all transactions
func (tr TransactionRequest) BuildURL() (endpoint string, err error) {
	nParams := countParams(tr.ForAccount, tr.ForLedger, tr.ForLiquidityPool, tr.forTransactionHash)

	if nParams > 1 {
		return endpoint, errors.New("invalid request: too many parameters")
	}

	endpoint = "transactions"
	if tr.ForAccount != "" {
		endpoint = fmt.Sprintf("accounts/%s/transactions", tr.ForAccount)
	}
	if tr.ForClaimableBalance != "" {
		endpoint = fmt.Sprintf("claimable_balances/%s/transactions", tr.ForClaimableBalance)
	}
	if tr.ForLedger > 0 {
		endpoint = fmt.Sprintf("ledgers/%d/transactions", tr.ForLedger)
	}
	if tr.ForLiquidityPool != "" {
		endpoint = fmt.Sprintf("liquidity_pools/%s/transactions", tr.ForLiquidityPool)
	}
	if tr.forTransactionHash != "" {
		endpoint = fmt.Sprintf("transactions/%s", tr.forTransactionHash)
	}

	queryParams := addQueryParams(cursor(tr.Cursor), limit(tr.Limit), tr.Order,
		includeFailed(tr.IncludeFailed))
	if queryParams != "" {
		endpoint = fmt.Sprintf("%s?%s", endpoint, queryParams)
	}

	_, err = url.Parse(endpoint)
	if err != nil {
		err = errors.Wrap(err, "failed to parse endpoint")
	}

	return endpoint, err
}

// HTTPRequest returns the http request for the transactions endpoint
func (tr TransactionRequest) HTTPRequest(orbitrURL string) (*http.Request, error) {
	endpoint, err := tr.BuildURL()
	if err != nil {
		return nil, err
	}

	return http.NewRequest("GET", orbitrURL+endpoint, nil)
}

// TransactionHandler is a function that is called when a new transaction is received
type TransactionHandler func(hProtocol.Transaction)

// StreamTransactions streams executed transactions. It can be used to stream all transactions and  transactions for an account. Use context.WithCancel to stop streaming or context.Background() if you want
// to stream indefinitely. TransactionHandler is a user-supplied function that is executed for each streamed transaction received.
func (tr TransactionRequest) StreamTransactions(ctx context.Context, client *Client,
	handler TransactionHandler) (err error) {
	endpoint, err := tr.BuildURL()
	if err != nil {
		return errors.Wrap(err, "unable to build endpoint")
	}

	url := fmt.Sprintf("%s%s", client.fixOrbitRURL(), endpoint)

	return client.stream(ctx, url, func(data []byte) error {
		var transaction hProtocol.Transaction
		err = json.Unmarshal(data, &transaction)
		if err != nil {
			return errors.Wrap(err, "error unmarshaling data")
		}
		handler(transaction)
		return nil
	})
}
