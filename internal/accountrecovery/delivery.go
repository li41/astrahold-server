package accountrecovery

import (
	"context"
	"errors"
	"time"
)

const MaxDeliveryDestinationBytes = 128

var (
	ErrDeliveryTransient = errors.New("accountrecovery: delivery transient failure")
	ErrDeliveryPermanent = errors.New("accountrecovery: delivery permanent failure")
)

// Delivery is Server-owned challenge material passed to a recovery transport.
// Destination ownership is resolved by Server/provider configuration, never by
// the public recovery request. Adapters must not log Proof or RequestID.
type Delivery struct {
	RequestID   string
	Destination string
	Proof       []byte
	ExpiresAt   time.Time
}

func (d Delivery) Valid() bool {
	return validTrimmed(d.RequestID, MaxRequestIDBytes) &&
		validTrimmed(d.Destination, MaxDeliveryDestinationBytes) &&
		len(d.Proof) > 0 && len(d.Proof) <= MaxProofBytes &&
		!d.ExpiresAt.IsZero()
}

// DeliveryAdapter owns transport only. It does not decide account eligibility,
// verify a proof, create a recovery Grant, or mutate durable account state.
type DeliveryAdapter interface {
	Deliver(context.Context, Delivery) error
	Method() string
	Revision() string
}
