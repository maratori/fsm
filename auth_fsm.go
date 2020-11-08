package fsm

import (
	"fmt"
	"strings"
)

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
type Callback func(*Memory) error
type Condition func(Memory) bool

type AuthFSMDefinition struct {
	InitialState             State
	InitialMemory            Memory
	Callbacks                map[State]Callback
	EventTransitions         map[State]map[Event]State
	ConditionalTransitions   map[State]map[State]Condition
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
	AuthPending         State = "AuthPending" // TODO: send pending state to GPM
	AuthRetry           State = "AuthRetry"
	AuthSucceeded       State = "AuthSucceeded"
	AuthWaitForRetry    State = "AuthWaitForRetry"
	CheckAuthStatus     State = "CheckAuthStatus"
	CheckPaymentStatus  State = "CheckPaymentStatus"
	Failed              State = "Failed"
	New                 State = "New"
	PaymentCreated      State = "PaymentCreated"
	PaymentFailed       State = "PaymentFailed" // TODO: payment can't have failed status
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
	CreateAuth(*Memory) error
	CreatePayment(*Memory) error
	GetAuthStatusFromZooz(*Memory) error
	GetPaymentStatusFromZooz(*Memory) error
	ScheduleJob(*Memory) error
	SendErrorToGPM(*Memory) error
	SendSuccessToGPM(*Memory) error
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
		Callbacks: map[State]Callback{
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
		ConditionalTransitions: map[State]map[State]Condition{
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

func (d AuthFSMDefinition) Validate() error {
	for from := range d.EventTransitions {
		if _, ok := d.ConditionalTransitions[from]; ok {
			return fmt.Errorf("different transition types from %q", from)
		}
		if _, ok := d.UnconditionalTransitions[from]; ok {
			return fmt.Errorf("different transition types from %q", from)
		}
	}
	for from := range d.ConditionalTransitions {
		if _, ok := d.UnconditionalTransitions[from]; ok {
			return fmt.Errorf("different transition types from %q", from)
		}
	}

	if _, ok := d.EventTransitions[d.InitialState]; !ok {
		return fmt.Errorf("initial state %q should be permanent", d.InitialState)
	}

	for from, to := range d.UnconditionalTransitions {
		if from == to {
			return fmt.Errorf("unconditional transition from %q", from)
		}
	}

	// TODO: check graph connectivity (that all states can be reached from initial)
	// TODO: check that all callbacks are defined on reachable states

	return nil
}

func (d AuthFSMDefinition) New() *AuthFSMInstance {
	allStates := map[State]struct{}{}
	permanentStates := map[State]struct{}{}
	allEvents := map[Event]struct{}{}

	callbacks := map[State]Callback{}
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

	conditionalTransitions := map[State]map[State]Condition{}
	for from, transitions := range d.ConditionalTransitions {
		allStates[from] = struct{}{}
		transitionsCopy := map[State]Condition{}
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

func (d AuthFSMDefinition) Restore(current State, memory Memory) (*AuthFSMInstance, error) {
	a := d.New()
	a.Current = current
	a.Memory = &memory
	if _, ok := a.PermanentStates[current]; !ok {
		return nil, fmt.Errorf("can restore: state %q should be permanent", current)
	}
	return a, nil
}

func (a *AuthFSMInstance) ProcessEvent(event Event) error {
	if _, ok := a.PermanentStates[a.Current]; !ok {
		return fmt.Errorf("current state %q is not permanent", a.Current)
	}

	if _, ok := a.Events[event]; !ok {
		return fmt.Errorf("unknown event %q", event)
	}

	newState, ok := a.Definition.EventTransitions[a.Current][event]
	if !ok {
		return fmt.Errorf("no transition from %q for event %q", a.Current, event)
	}

	err := a.switchTo(newState)
	if err != nil {
		return err
	}

	return a.goToNextPermanentState()
}

func (a *AuthFSMInstance) goToNextPermanentState() error {
	for {
		if _, ok := a.PermanentStates[a.Current]; ok {
			return nil
		}

		if newState, ok := a.Definition.UnconditionalTransitions[a.Current]; ok {
			err := a.switchTo(newState)
			if err != nil {
				return err
			}
			continue
		}

		if transitions, ok := a.Definition.ConditionalTransitions[a.Current]; ok {
			var newState []State
			for candidate, cond := range transitions {
				if cond(*a.Memory) {
					newState = append(newState, candidate)
				}
			}
			switch len(newState) {
			case 1:
				err := a.switchTo(newState[0])
				if err != nil {
					return err
				}
			case 0:
				return fmt.Errorf("all conditional transitions returned false from %q", a.Current)
			default:
				x := make([]string, 0, len(newState))
				for _, n := range newState {
					x = append(x, fmt.Sprintf("%q->%q", a.Current, n))
				}
				return fmt.Errorf("%d transactions possible: %s", len(newState), strings.Join(x, ", "))
			}
			continue
		}

		panic("should never happen")
	}
	return nil
}

func (a *AuthFSMInstance) switchTo(newState State) error {
	if fn, ok := a.Definition.Callbacks[newState]; ok {
		err := fn(a.Memory)
		if err != nil {
			return fmt.Errorf("on enter to %q from %q: %w", newState, a.Current, err)
		}
	}
	a.Current = newState
	return nil
}

func canRetry(err string) bool {
	return err == "can retry"
}
