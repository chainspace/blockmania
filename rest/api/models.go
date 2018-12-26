package api

// TransactionRequest represent the payload for a single transasction,
// the byte slice will be a base64 encoded string in the json payload
type TransactionRequest struct {
	Tx []byte `json:"tx"`
}

// TransactionsRequest represent the payload for a list of transasctions
// the byte slice slice will be a base64 encoded string slice in the json payload
type TransactionsRequest struct {
	Txs [][]byte `json:"txs"`
}

type Response struct {
	Error string `json:"error,omitempty"`
}
