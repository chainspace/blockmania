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
	return s.node.Broadcast.AddTransaction(tx, 0)
}

func (s *service) AddTransactions(txs [][]byte) error {
	for _, tx := range txs {
		err := s.node.Broadcast.AddTransaction(tx, 0)
		if err != nil {
			return err
		}
	}
	return nil
}
