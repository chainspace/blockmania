// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"
import network "github.com/chainspace/blockmania/network"
import signature "github.com/chainspace/blockmania/internal/crypto/signature"
import time "time"

// NetTopology is an autogenerated mock type for the NetTopology type
type NetTopology struct {
	mock.Mock
}

// Dial provides a mock function with given fields: nodeID, timeout
func (_m *NetTopology) Dial(nodeID uint64, timeout time.Duration) (*network.Conn, error) {
	ret := _m.Called(nodeID, timeout)

	var r0 *network.Conn
	if rf, ok := ret.Get(0).(func(uint64, time.Duration) *network.Conn); ok {
		r0 = rf(nodeID, timeout)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*network.Conn)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(uint64, time.Duration) error); ok {
		r1 = rf(nodeID, timeout)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SeedPublicKeys provides a mock function with given fields:
func (_m *NetTopology) SeedPublicKeys() map[uint64]signature.PublicKey {
	ret := _m.Called()

	var r0 map[uint64]signature.PublicKey
	if rf, ok := ret.Get(0).(func() map[uint64]signature.PublicKey); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[uint64]signature.PublicKey)
		}
	}

	return r0
}

// TotalNodes provides a mock function with given fields:
func (_m *NetTopology) TotalNodes() uint64 {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}
