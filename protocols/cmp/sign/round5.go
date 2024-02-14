package sign

import (
	"errors"

	"github.com/mr-shifu/mpc-lib/lib/round"
	"github.com/mr-shifu/mpc-lib/pkg/ecdsa"
	"github.com/mr-shifu/mpc-lib/pkg/math/curve"
	"github.com/mr-shifu/mpc-lib/pkg/party"
)

var _ round.Round = (*round5)(nil)

type round5 struct {
	*round4

	// SigmaShares[j] = σⱼ = m⋅kⱼ + χⱼ⋅R|ₓ
	SigmaShares map[party.ID]curve.Scalar

	// Delta = δ = ∑ⱼ δⱼ
	// computed from received shares
	Delta curve.Scalar

	// BigDelta = Δ = ∑ⱼ Δⱼ
	BigDelta curve.Point

	// R = [δ⁻¹] Γ
	BigR curve.Point

	// R = R|ₓ
	R curve.Scalar

	// Number of Broacasted Messages received
	MessageBroadcasted map[party.ID]bool
}

type broadcast5 struct {
	round.NormalBroadcastContent
	SigmaShare curve.Scalar
}

// StoreBroadcastMessage implements round.BroadcastRound.
//
// - save σⱼ
func (r *round5) StoreBroadcastMessage(msg round.Message) error {
	body, ok := msg.Content.(*broadcast5)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	if body.SigmaShare.IsZero() {
		return round.ErrNilFields
	}

	r.SigmaShares[msg.From] = body.SigmaShare

	// Mark the message as received
	r.MessageBroadcasted[msg.From] = true

	return nil
}

// VerifyMessage implements round.Round.
func (round5) VerifyMessage(round.Message) error { return nil }

// StoreMessage implements round.Round.
func (round5) StoreMessage(round.Message) error { return nil }

// Finalize implements round.Round
//
// - compute σ = ∑ⱼ σⱼ
// - verify signature.
func (r *round5) Finalize(chan<- *round.Message) (round.Session, error) {
	// Verify if all parties commitments are received
	if len(r.MessageBroadcasted) != r.N()-1 {
		return nil, round.ErrNotEnoughMessages
	}

	// compute σ = ∑ⱼ σⱼ
	Sigma := r.Group().NewScalar()
	for _, j := range r.PartyIDs() {
		Sigma.Add(r.SigmaShares[j])
	}

	signature := &ecdsa.Signature{
		R: r.BigR,
		S: Sigma,
	}

	if !signature.Verify(r.PublicKey, r.Message) {
		return r.AbortRound(errors.New("failed to validate signature")), nil
	}

	return r.ResultRound(signature), nil
}

func (r *round5) CanFinalize() bool {
	// Verify if all parties commitments are received
	return len(r.MessageBroadcasted) == r.N()-1
}

// MessageContent implements round.Round.
func (r *round5) MessageContent() round.Content { return nil }

// RoundNumber implements round.Content.
func (broadcast5) RoundNumber() round.Number { return 5 }

// BroadcastContent implements round.BroadcastRound.
func (r *round5) BroadcastContent() round.BroadcastContent {
	return &broadcast5{
		SigmaShare: r.Group().NewScalar(),
	}
}

// Number implements round.Round.
func (round5) Number() round.Number { return 5 }

func (r *round5) Equal(other round.Round) bool {
	return true
}
