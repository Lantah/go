package gravity

// InfoResponse is the json response returned from gravity's /info
// endpoint.
type InfoResponse struct {
	Info struct {
		Build           string     `json:"build"`
		Network         string     `json:"network"`
		ProtocolVersion int        `json:"protocol_version"`
		State           string     `json:"state"`
		Ledger          LedgerInfo `json:"ledger"`

		// TODO: all the other fields
	}
}

// LedgerInfo is the part of the gravity's info json response.
// It's returned under `ledger` key
type LedgerInfo struct {
	Age          int    `json:"age"`
	BaseFee      int    `json:"baseFee"`
	BaseReserve  int    `json:"baseReserve"`
	CloseTime    int    `json:"closeTime"`
	Hash         string `json:"hash"`
	MaxTxSetSize int    `json:"maxTxSetSize"`
	Num          int    `json:"num"`
	Version      int    `json:"version"`
}

// IsSynced returns a boolean indicating whether gravity is synced with the
// network.
func (resp *InfoResponse) IsSynced() bool {
	return resp.Info.State == "Synced!"
}
