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

type AuthFSMDefinition struct {
	InitialState             State
	InitialMemory            Memory
	Callbacks                map[State]func(*Memory)
	EventTransitions         map[State]map[Event]State
	ConditionalTransitions   map[State]map[State]func(Memory) bool
	UnconditionalTransitions map[State]State
}

type AuthFSMInstance struct {
	Definition AuthFSMDefinition

	Current         State
	Memory          *Memory
	AllStates       map[State]struct{}
	PermanentStates map[State]struct{}
	Events          map[Event]struct{}
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

func NewAuthFSMDefinition(c Callbacks) AuthFSMDefinition {
	return AuthFSMDefinition{
		InitialState: Initial,
		InitialMemory: Memory{
			PaymentStatus:   "",
			PaymentError:    "",
			PaymentAttempts: 0,
			AuthStatus:      "",
			AuthError:       "",
			AuthAttempts:    0,
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

func (d AuthFSMDefinition) Validate() {
	for from := range d.EventTransitions {
		if _, ok := d.ConditionalTransitions[from]; ok {
			panic(fmt.Sprintf("different transition types from %q", from))
		}
		if _, ok := d.UnconditionalTransitions[from]; ok {
			panic(fmt.Sprintf("different transition types from %q", from))
		}
	}
	for from := range d.ConditionalTransitions {
		if _, ok := d.UnconditionalTransitions[from]; ok {
			panic(fmt.Sprintf("different transition types from %q", from))
		}
	}

	if _, ok := d.EventTransitions[d.InitialState]; !ok {
		panic(fmt.Sprintf("initial state %q should be permanent", d.InitialState))
	}

	for from, to := range d.UnconditionalTransitions {
		if from == to {
			panic(fmt.Sprintf("unconditional transition from %q", from))
		}
	}

	// TODO: check graph connectivity (that all states can be reached from initial)
	// TODO: check that all callbacks are defined on reachable states
}

func (d AuthFSMDefinition) New() *AuthFSMInstance {
	allStates := map[State]struct{}{}
	permanentStates := map[State]struct{}{}
	allEvents := map[Event]struct{}{}

	callbacks := map[State]func(*Memory){}
	for from, fn := range d.Callbacks {
		allStates[from] = struct{}{}
		if fn != nil {
			callbacks[from] = fn
		}
	}

	eventTransitions := map[State]map[Event]State{}
	for from, events := range d.EventTransitions {
		allStates[from] = struct{}{}
		permanentStates[from] = struct{}{}
		eventsCopy := map[Event]State{}
		for event, to := range events {
			allStates[to] = struct{}{}
			allEvents[event] = struct{}{}
			eventsCopy[event] = to
		}
		if len(eventsCopy) > 0 {
			eventTransitions[from] = eventsCopy
		}
	}

	conditionalTransitions := map[State]map[State]func(Memory) bool{}
	for from, transitions := range d.ConditionalTransitions {
		allStates[from] = struct{}{}
		transitionsCopy := map[State]func(Memory) bool{}
		for to, cond := range transitions {
			allStates[to] = struct{}{}
			if cond != nil {
				transitionsCopy[to] = cond
			}
		}
		if len(transitionsCopy) > 0 {
			conditionalTransitions[from] = transitionsCopy
		}
	}

	unconditionalTransitions := map[State]State{}
	for from, to := range d.UnconditionalTransitions {
		allStates[from] = struct{}{}
		allStates[to] = struct{}{}
		unconditionalTransitions[from] = to
	}

	currentMemory := d.InitialMemory

	return &AuthFSMInstance{
		Definition: AuthFSMDefinition{
			InitialState:             d.InitialState,
			InitialMemory:            d.InitialMemory,
			Callbacks:                callbacks,
			EventTransitions:         eventTransitions,
			ConditionalTransitions:   conditionalTransitions,
			UnconditionalTransitions: unconditionalTransitions,
		},
		Current:         d.InitialState,
		Memory:          &currentMemory,
		AllStates:       allStates,
		PermanentStates: permanentStates,
		Events:          allEvents,
	}
}

func (d AuthFSMDefinition) Restore(current State, memory Memory) *AuthFSMInstance {
	a := d.New()
	a.Current = current
	a.Memory = &memory
	if _, ok := a.PermanentStates[current]; !ok {
		panic(fmt.Sprintf("can restore: state %q should be permanent", current))
	}
	return a
}

func canRetry(err string) bool {
	return err == "can retry"
}
