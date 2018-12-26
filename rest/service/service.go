package service

import "chainspace.io/blockmania/node"

type service struct {
	node *node.Server
}

type Service interface {
	AddTransaction([]byte) error
	AddTransactions([][]byte) error
}

func New(node *node.Server) Service {
	return &service{node: node}
}

func (s *service) AddTransaction(tx []byte) error {
	return nil
}

func (s *service) AddTransactions(txs [][]byte) error {
	return nil
}
