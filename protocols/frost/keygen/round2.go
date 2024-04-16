package keygen

import (
	"errors"

	"github.com/mr-shifu/mpc-lib/core/hash"
	"github.com/mr-shifu/mpc-lib/core/math/curve"
	"github.com/mr-shifu/mpc-lib/core/math/polynomial"
	"github.com/mr-shifu/mpc-lib/lib/round"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/commitment"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/ecdsa"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/rid"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/vss"
	"github.com/mr-shifu/mpc-lib/pkg/keyopts"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/common/config"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/common/message"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/common/state"
)

type broadcast2 struct {
	round.ReliableBroadcastContent
	VSSPolynomial *polynomial.Exponent

	SchnorrCommitment curve.Point
	SchnorrProof      curve.Scalar

	Commitment hash.Commitment
}

type round2 struct {
	*round.Helper

	configmgr   config.KeyConfigManager
	statemanger state.MPCStateManager
	msgmgr      message.MessageManager
	bcstmgr     message.MessageManager
	ec_km       ecdsa.ECDSAKeyManager
	ec_vss_km   ecdsa.ECDSAKeyManager
	vss_mgr     vss.VssKeyManager
	chainKey_km rid.RIDManager
	commit_mgr  commitment.CommitmentManager
}

// StoreBroadcastMessage implements round.BroadcastRound.
func (r *round2) StoreBroadcastMessage(msg round.Message) error {
	from := msg.From
	body, ok := msg.Content.(*broadcast2)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	// validate broadcast params
	if !body.SchnorrCommitment.IsIdentity() {
		return errors.New("frost.Keygen.Round2: invalid schnorr commitment")
	}
	if body.SchnorrProof.IsZero() {
		return errors.New("frost.Keygen.Round2: invalid schnorr proof")
	}
	if body.VSSPolynomial == nil {
		return errors.New("frost.Keygen.Round2: invalid VSS polynomial")
	}

	fromOpts := keyopts.Options{}
	fromOpts.Set("id", r.ID, "partyid", string(from))

	// validate commitment and import it to commitment store
	if err := body.Commitment.Validate(); err != nil {
		return err
	}
	cmt := r.commit_mgr.NewCommitment(body.Commitment, nil)
	if err := r.commit_mgr.Import(cmt, fromOpts); err != nil {
		return err
	}

	// ToDo we must be able to first verify schnorr proof before importing commitment
	// Import Party Public Key and VSS Exponents
	pk := body.VSSPolynomial.Constant()
	k := r.ec_km.NewKey(nil, pk, r.Group())
	ecKey, err := r.ec_km.ImportKey(k, fromOpts)
	if err != nil {
		return err
	}

	// ToDo it's better to verify proof without importing commitment
	// verify schnorr proof
	if err := ecKey.ImportSchnorrCommitment(body.SchnorrCommitment); err != nil {
		return err
	}
	verified, err := ecKey.VerifySchnorrProof(r.Helper.HashForID(from), body.SchnorrProof)
	if err != nil {
		return err
	}
	if !verified {
		return errors.New("frost.Keygen.Round2: schnorr proof verification failed")
	}

	return nil
}

// VerifyMessage implements round.Round.
func (round2) VerifyMessage(round.Message) error { return nil }

// StoreMessage implements round.Round.
func (round2) StoreMessage(round.Message) error { return nil }

// RoundNumber implements round.Content.
func (broadcast2) RoundNumber() round.Number { return 2 }