// Code generated by mockery v2.7.4. DO NOT EDIT.

package mocks

import (
	io "io"

	mock "github.com/stretchr/testify/mock"
)

// Creds is an autogenerated mock type for the Creds type
type Creds struct {
	mock.Mock
}

// Environ provides a mock function with given fields:
func (_m *Creds) Environ() (io.Closer, []string, error) {
	ret := _m.Called()

	var r0 io.Closer
	if rf, ok := ret.Get(0).(func() io.Closer); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.Closer)
		}
	}

	var r1 []string
	if rf, ok := ret.Get(1).(func() []string); ok {
		r1 = rf()
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]string)
		}
	}

	var r2 error
	if rf, ok := ret.Get(2).(func() error); ok {
		r2 = rf()
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}
