package fsm

import "fmt"

type Memory struct {
	PaymentStatus   string
	PaymentError    string
	PaymentAttempts int
	AuthStatus      string
	AuthError       string
	AuthAttempts    int
}

type State string
type Event string

type AuthFSM struct {
	Current State
	Memory  *Memory

	States                   map[State]struct{}
	Events                   map[Event]struct{}
	Callbacks                map[State]func(*Memory)
	EventTransitions         map[State]map[Event]State
	ConditionalTransitions   map[State]map[State]func(Memory) bool
	UnconditionalTransitions map[State]State
}

const (
	Initial State = "Initial"

	AuthCreated         State = "AuthCreated"
	AuthFailed          State = "AuthFailed"
	AuthPending         State = "AuthPending"
	AuthRetry           State = "AuthRetry"
	AuthSucceeded       State = "AuthSucceeded"
	AuthWaitForRetry    State = "AuthWaitForRetry"
	CheckAuthStatus     State = "CheckAuthStatus"
	CheckPaymentStatus  State = "CheckPaymentStatus"
	Failed              State = "Failed"
	New                 State = "New"
	PaymentCreated      State = "PaymentCreated"
	PaymentFailed       State = "PaymentFailed"
	PaymentPending      State = "PaymentPending"
	PaymentRetry        State = "PaymentRetry"
	PaymentSucceeded    State = "PaymentSucceeded"
	PaymentWaitForRetry State = "PaymentWaitForRetry"
	SendingErrorToGPM   State = "SendingErrorToGPM"
	SendingSuccessToGPM State = "SendingSuccessToGPM"
	Succeeded           State = "Succeeded"
)

const (
	AuthWebhookFromZooz    Event = "AuthWebhookFromZooz"
	Job                    Event = "Job"
	PaymentWebhookFromZooz Event = "PaymentWebhookFromZooz"
	RequestFromGPM         Event = "RequestFromGPM"
)

const (
	MaxAuthAttempts    = 5
	MaxPaymentAttempts = 5
)

type Callbacks interface {
	CreateAuth(*Memory)
	CreatePayment(*Memory)
	GetAuthStatusFromZooz(*Memory)
	GetPaymentStatusFromZooz(*Memory)
	ScheduleJob(*Memory)
	SendErrorToGPM(*Memory)
	SendSuccessToGPM(*Memory)
}

func NewAuthFSM(c Callbacks) *AuthFSM {
	return &AuthFSM{
		Current: Initial,
		Memory: &Memory{
			PaymentStatus:   "",
			PaymentError:    "",
			PaymentAttempts: 0,
			AuthStatus:      "",
			AuthError:       "",
			AuthAttempts:    0,
		},
		States: map[State]struct{}{
			Initial:             {},
			AuthCreated:         {},
			AuthFailed:          {},
			AuthPending:         {},
			AuthRetry:           {},
			AuthSucceeded:       {},
			AuthWaitForRetry:    {},
			CheckAuthStatus:     {},
			CheckPaymentStatus:  {},
			Failed:              {},
			New:                 {},
			PaymentCreated:      {},
			PaymentFailed:       {},
			PaymentPending:      {},
			PaymentRetry:        {},
			PaymentSucceeded:    {},
			PaymentWaitForRetry: {},
			SendingErrorToGPM:   {},
			SendingSuccessToGPM: {},
			Succeeded:           {},
		},
		Events: map[Event]struct{}{
			AuthWebhookFromZooz:    {},
			Job:                    {},
			PaymentWebhookFromZooz: {},
			RequestFromGPM:         {},
		},
		Callbacks: map[State]func(*Memory){
			AuthPending:         c.ScheduleJob,
			AuthRetry:           c.CreateAuth,
			AuthWaitForRetry:    c.ScheduleJob,
			CheckAuthStatus:     c.GetAuthStatusFromZooz,
			CheckPaymentStatus:  c.GetPaymentStatusFromZooz,
			New:                 c.CreatePayment,
			PaymentPending:      c.ScheduleJob,
			PaymentRetry:        c.CreatePayment,
			PaymentSucceeded:    c.CreateAuth,
			PaymentWaitForRetry: c.ScheduleJob,
			SendingErrorToGPM:   c.SendErrorToGPM,
			SendingSuccessToGPM: c.SendSuccessToGPM,
		},
		EventTransitions: map[State]map[Event]State{
			Initial: {
				RequestFromGPM: New,
			},
			AuthPending: {
				Job:                 CheckAuthStatus,
				AuthWebhookFromZooz: AuthCreated,
				RequestFromGPM:      CheckAuthStatus,
			},
			AuthWaitForRetry: {
				Job:            AuthRetry,
				RequestFromGPM: AuthRetry,
			},
			Failed: {
				RequestFromGPM: SendingErrorToGPM,
			},
			PaymentPending: {
				Job:                    CheckPaymentStatus,
				PaymentWebhookFromZooz: PaymentCreated,
				RequestFromGPM:         CheckPaymentStatus,
			},
			PaymentWaitForRetry: {
				Job:            PaymentRetry,
				RequestFromGPM: PaymentRetry,
			},
			Succeeded: {
				RequestFromGPM: SendingSuccessToGPM,
			},
		},
		ConditionalTransitions: map[State]map[State]func(Memory) bool{
			AuthCreated: {
				AuthFailed:    func(m Memory) bool { return m.AuthStatus == "failed" },
				AuthPending:   func(m Memory) bool { return m.AuthStatus == "pending" },
				AuthSucceeded: func(m Memory) bool { return m.AuthStatus == "succeeded" },
			},
			AuthFailed: {
				AuthWaitForRetry:  func(m Memory) bool { return canRetry(m.AuthError) && m.AuthAttempts < MaxAuthAttempts },
				SendingErrorToGPM: func(m Memory) bool { return !canRetry(m.AuthError) || m.AuthAttempts >= MaxAuthAttempts },
			},
			PaymentCreated: {
				PaymentFailed:    func(m Memory) bool { return m.PaymentStatus == "failed" },
				PaymentPending:   func(m Memory) bool { return m.PaymentStatus == "pending" },
				PaymentSucceeded: func(m Memory) bool { return m.PaymentStatus == "succeeded" },
			},
			PaymentFailed: {
				PaymentWaitForRetry: func(m Memory) bool { return canRetry(m.PaymentError) && m.PaymentAttempts < MaxPaymentAttempts },
				SendingErrorToGPM:   func(m Memory) bool { return !canRetry(m.PaymentError) || m.PaymentAttempts >= MaxPaymentAttempts },
			},
		},
		UnconditionalTransitions: map[State]State{
			AuthRetry:           AuthCreated,
			AuthSucceeded:       SendingSuccessToGPM,
			CheckAuthStatus:     AuthCreated,
			CheckPaymentStatus:  PaymentCreated,
			New:                 PaymentCreated,
			PaymentRetry:        PaymentCreated,
			SendingErrorToGPM:   Failed,
			SendingSuccessToGPM: Succeeded,
		},
	}
}

func (m *AuthFSM) Validate() {
	m.validateState(m.Current)
	for state, fn := range m.Callbacks {
		m.validateState(state)
		if fn == nil {
			panic(fmt.Sprintf("nil callback for state %q", state))
		}
	}
	transitionsFrom := map[State]struct{}{}
	rememberTransitionFrom := func(state State) {
		_, ok := transitionsFrom[state]
		if ok {
			panic(fmt.Sprintf("repeated transition from state %q", state))
		}
		transitionsFrom[state] = struct{}{}
	}
	for state, transitions := range m.EventTransitions {
		m.validateState(state)
		for event, newState := range transitions {
			m.validateEvent(event)
			m.validateState(newState)
		}
		rememberTransitionFrom(state)
	}
	for state, conditions := range m.ConditionalTransitions {
		m.validateState(state)
		for newState, condFn := range conditions {
			m.validateState(newState)
			if condFn == nil {
				panic(fmt.Sprintf("nil condition for transition %q -> %q", state, newState))
			}
		}
		rememberTransitionFrom(state)
	}
	for state, newState := range m.UnconditionalTransitions {
		m.validateState(state)
		m.validateState(newState)
		rememberTransitionFrom(state)
	}
}

func (m *AuthFSM) validateState(state State) {
	_, ok := m.States[state]
	if !ok {
		panic(fmt.Sprintf("state %q is not in list of states", state))
	}
}

func (m *AuthFSM) validateEvent(event Event) {
	_, ok := m.Events[event]
	if !ok {
		panic(fmt.Sprintf("event %q is not in list of events", event))
	}
}

func canRetry(err string) bool {
	return err == "can retry"
}
