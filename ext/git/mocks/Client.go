// Code generated by mockery v2.7.4. DO NOT EDIT.

package mocks

import (
	git "github.com/argoproj-labs/argocd-image-updater/ext/git"
	mock "github.com/stretchr/testify/mock"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Add provides a mock function with given fields: path
func (_m *Client) Add(path string) error {
	ret := _m.Called(path)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(path)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Branch provides a mock function with given fields: sourceBranch, targetBranch
func (_m *Client) Branch(sourceBranch string, targetBranch string) error {
	ret := _m.Called(sourceBranch, targetBranch)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(sourceBranch, targetBranch)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Checkout provides a mock function with given fields: revision
func (_m *Client) Checkout(revision string) error {
	ret := _m.Called(revision)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(revision)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Commit provides a mock function with given fields: pathSpec, opts
func (_m *Client) Commit(pathSpec string, opts *git.CommitOptions) error {
	ret := _m.Called(pathSpec, opts)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, *git.CommitOptions) error); ok {
		r0 = rf(pathSpec, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CommitSHA provides a mock function with given fields:
func (_m *Client) CommitSHA() (string, error) {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Config provides a mock function with given fields: username, email
func (_m *Client) Config(username string, email string) error {
	ret := _m.Called(username, email)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(username, email)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Fetch provides a mock function with given fields:
func (_m *Client) Fetch() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// FetchRef provides a mock function with given fields: ref
func (_m *Client) FetchRef(ref string) error {
	ret := _m.Called(ref)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(ref)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Init provides a mock function with given fields:
func (_m *Client) Init() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// LsFiles provides a mock function with given fields: path
func (_m *Client) LsFiles(path string) ([]string, error) {
	ret := _m.Called(path)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string) []string); ok {
		r0 = rf(path)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(path)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LsLargeFiles provides a mock function with given fields:
func (_m *Client) LsLargeFiles() ([]string, error) {
	ret := _m.Called()

	var r0 []string
	if rf, ok := ret.Get(0).(func() []string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LsRefs provides a mock function with given fields:
func (_m *Client) LsRefs() (*git.Refs, error) {
	ret := _m.Called()

	var r0 *git.Refs
	if rf, ok := ret.Get(0).(func() *git.Refs); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*git.Refs)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LsRemote provides a mock function with given fields: revision
func (_m *Client) LsRemote(revision string) (string, error) {
	ret := _m.Called(revision)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(revision)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(revision)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Push provides a mock function with given fields: remote, branch, force
func (_m *Client) Push(remote string, branch string, force bool) error {
	ret := _m.Called(remote, branch, force)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, bool) error); ok {
		r0 = rf(remote, branch, force)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RevisionMetadata provides a mock function with given fields: revision
func (_m *Client) RevisionMetadata(revision string) (*git.RevisionMetadata, error) {
	ret := _m.Called(revision)

	var r0 *git.RevisionMetadata
	if rf, ok := ret.Get(0).(func(string) *git.RevisionMetadata); ok {
		r0 = rf(revision)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*git.RevisionMetadata)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(revision)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Root provides a mock function with given fields:
func (_m *Client) Root() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// SymRefToBranch provides a mock function with given fields: symRef
func (_m *Client) SymRefToBranch(symRef string) (string, error) {
	ret := _m.Called(symRef)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(symRef)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(symRef)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// VerifyCommitSignature provides a mock function with given fields: _a0
func (_m *Client) VerifyCommitSignature(_a0 string) (string, error) {
	ret := _m.Called(_a0)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
