package fsm_test

import (
	"testing"

	"github.com/maratori/fsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFSM(t *testing.T) {
	callbacks := &MockCallbacks{}
	callbacks.Test(t)
	def := fsm.NewAuthFSMDefinition(callbacks)
	err := def.Validate()
	require.NoError(t, err)
	auth := def.New()
	assert.Equal(t, fsm.Initial, auth.Current)
	err = auth.ProcessEvent(fsm.Job)
	assert.EqualError(t, err, `no transition from "Initial" for event "Job"`)
	callbacks.On("CreatePayment", &fsm.Memory{}).Run(func(args mock.Arguments) { args.Get(0).(*fsm.Memory).PaymentStatus = "pending" }).Return(nil).Once()
	callbacks.On("ScheduleJob", &fsm.Memory{PaymentStatus: "pending"}).Return(nil).Once()
	err = auth.ProcessEvent(fsm.RequestFromGPM)
	assert.NoError(t, err)
	callbacks.AssertExpectations(t)
}

type MockCallbacks struct {
	mock.Mock
}

func (m *MockCallbacks) CreateAuth(mem *fsm.Memory) error               { return m.Called(mem).Error(0) }
func (m *MockCallbacks) CreatePayment(mem *fsm.Memory) error            { return m.Called(mem).Error(0) }
func (m *MockCallbacks) GetAuthStatusFromZooz(mem *fsm.Memory) error    { return m.Called(mem).Error(0) }
func (m *MockCallbacks) GetPaymentStatusFromZooz(mem *fsm.Memory) error { return m.Called(mem).Error(0) }
func (m *MockCallbacks) ScheduleJob(mem *fsm.Memory) error              { return m.Called(mem).Error(0) }
func (m *MockCallbacks) SendErrorToGPM(mem *fsm.Memory) error           { return m.Called(mem).Error(0) }
func (m *MockCallbacks) SendSuccessToGPM(mem *fsm.Memory) error         { return m.Called(mem).Error(0) }
