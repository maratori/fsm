package fsm

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
	ConditionalTransitions   map[State]func(Memory) State
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
		ConditionalTransitions: map[State]func(Memory) State{
			AuthCreated: func(m Memory) State {
				switch m.AuthStatus {
				case "failed":
					return AuthFailed
				case "pending":
					return AuthPending
				case "succeeded":
					return AuthSucceeded
				default:
					panic(m.AuthStatus)
				}
			},
			AuthFailed: func(m Memory) State {
				switch {
				case m.AuthAttempts < MaxAuthAttempts && canRetry(m.AuthError):
					return AuthWaitForRetry
				default:
					return SendingErrorToGPM
				}
			},
			PaymentCreated: func(m Memory) State {
				switch m.PaymentStatus {
				case "failed":
					return PaymentFailed
				case "pending":
					return PaymentPending
				case "succeeded":
					return PaymentSucceeded
				default:
					panic(m.PaymentStatus)
				}
			},
			PaymentFailed: func(m Memory) State {
				switch {
				case m.PaymentAttempts < MaxPaymentAttempts && canRetry(m.PaymentError):
					return PaymentWaitForRetry
				default:
					return SendingErrorToGPM
				}
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

func canRetry(err string) bool {
	return err == "can retry"
}
