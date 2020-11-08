package fsm_test

import (
	"testing"

	"github.com/maratori/fsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFSM(t *testing.T) {
	callbacks := &MockCallbacks{}
	callbacks.Test(t)
	auth := fsm.NewAuthFSM(callbacks)
	auth.Validate()
	assert.Equal(t, fsm.Initial, auth.Current)
	callbacks.AssertExpectations(t)
}

type MockCallbacks struct {
	mock.Mock
}

func (m *MockCallbacks) CreateAuth(mem *fsm.Memory)               { m.Called(mem) }
func (m *MockCallbacks) CreatePayment(mem *fsm.Memory)            { m.Called(mem) }
func (m *MockCallbacks) GetAuthStatusFromZooz(mem *fsm.Memory)    { m.Called(mem) }
func (m *MockCallbacks) GetPaymentStatusFromZooz(mem *fsm.Memory) { m.Called(mem) }
func (m *MockCallbacks) ScheduleJob(mem *fsm.Memory)              { m.Called(mem) }
func (m *MockCallbacks) SendErrorToGPM(mem *fsm.Memory)           { m.Called(mem) }
func (m *MockCallbacks) SendSuccessToGPM(mem *fsm.Memory)         { m.Called(mem) }
