package sign

import (
	"fmt"

	"github.com/mr-shifu/mpc-lib/core/math/curve"
	"github.com/mr-shifu/mpc-lib/core/math/sample"
	"github.com/mr-shifu/mpc-lib/core/party"
	"github.com/mr-shifu/mpc-lib/lib/round"
	"github.com/mr-shifu/mpc-lib/pkg/cryptosuite/sw/ecdsa"
	"github.com/mr-shifu/mpc-lib/pkg/cryptosuite/sw/hash"
	"github.com/mr-shifu/mpc-lib/pkg/cryptosuite/sw/vss"
	"github.com/mr-shifu/mpc-lib/pkg/keyopts"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/common/config"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/common/state"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/message"
)

// This round roughly corresponds with steps 3-6 of Figure 3 in the Frost paper:
//
//	https://eprint.iacr.org/2020/852.pdf
//
// The main differences stem from the lack of a signature authority.
//
// This means that instead of receiving a bundle of all the commitments, instead
// each participant sends us their commitment directly.
//
// Then, instead of sending our scalar response to the authority, we broadcast it
// to everyone instead.
type round2 struct {
	*round.Helper
	cfg        config.SignConfig
	statemgr   state.MPCStateManager
	msgmgr     message.MessageManager
	bcstmgr    message.MessageManager
	ecdsa_km   ecdsa.ECDSAKeyManager
	ec_vss_km  ecdsa.ECDSAKeyManager
	ec_sign_km ecdsa.ECDSAKeyManager
	vss_mgr    vss.VssKeyManager
	sign_d     ecdsa.ECDSAKeyManager
	sign_e     ecdsa.ECDSAKeyManager
	hash_mgr   hash.HashManager
}

type broadcast2 struct {
	round.ReliableBroadcastContent
	// D_i is the first commitment produced by the sender of this message.
	D curve.Point
	// E_i is the second commitment produced by the sender of this message.
	E curve.Point
}

// StoreBroadcastMessage implements round.BroadcastRound.
func (r *round2) StoreBroadcastMessage(msg round.Message) error {
	body, ok := msg.Content.(*broadcast2)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	if body.D.IsIdentity() || body.E.IsIdentity() {
		return fmt.Errorf("nonce commitment is the identity point")
	}

	opts := keyopts.Options{}
	opts.Set("id", r.ID, "partyid", string(msg.From))

	// store D params as EC Key into EC keystore
	dk := r.sign_d.NewKey(nil, body.D, r.Group())
	if _, err := r.sign_d.ImportKey(dk, opts); err != nil {
		return err
	}

	// store E params as EC Key into EC keystore
	ek := r.sign_e.NewKey(nil, body.E, r.Group())
	if _, err := r.sign_e.ImportKey(ek, opts); err != nil {
		return err
	}

	return nil
}

// VerifyMessage implements round.Round.
func (round2) VerifyMessage(round.Message) error { return nil }

// StoreMessage implements round.Round.
func (round2) StoreMessage(round.Message) error { return nil }

// Finalize implements round.Round.
func (r *round2) Finalize(out chan<- *round.Message) (round.Session, error) {
	rho := make(map[party.ID]curve.Scalar)

	// 0. fetch Dᵢ and Eᵢ from the keystore
	Ds := make(map[party.ID]curve.Point)
	Es := make(map[party.ID]curve.Point)
	for _, l := range r.PartyIDs() {
		opts := keyopts.Options{}
		opts.Set("id", r.ID, "partyid", string(l))
		dk, err := r.sign_d.GetKey(opts)
		if err != nil {
			return r, err
		}

		ek, err := r.sign_e.GetKey(opts)
		if err != nil {
			return r, err
		}

		Ds[l] = dk.PublicKeyRaw()
		Es[l] = ek.PublicKeyRaw()
	}

	// 1. generate random ρᵢ for each party i
	rhoPreHash := hash.New(nil)
	_ = rhoPreHash.WriteAny(r.cfg.Message())
	for _, l := range r.PartyIDs() {
		_ = rhoPreHash.WriteAny(Ds[l], Es[l])
	}
	for _, l := range r.PartyIDs() {
		rhoHash := rhoPreHash.Clone()
		_ = rhoHash.WriteAny(l)
		rho[l] = sample.Scalar(rhoHash.Digest(), r.Group())
	}

	// 2. Compute Rᵢ = (ρᵢ Eᵢ + Dᵢ) && R = Σᵢ Rᵢ
	R := r.Group().NewPoint()
	RShares := make(map[party.ID]curve.Point)
	for _, l := range r.PartyIDs() {
		RShares[l] = rho[l].Act(Es[l])
		RShares[l] = RShares[l].Add(Ds[l])
		R = R.Add(RShares[l])
	}

	// 3. Generate a random number as commitment to the nonce
	opts := keyopts.Options{}
	opts.Set("id", r.cfg.KeyID(), "partyid", string(r.SelfID()))
	ecKey, err := r.ecdsa_km.GetKey(opts)
	if err != nil {
		return r, err
	}
	cHash := hash.New(nil)
	_ = cHash.WriteAny(R, ecKey.PublicKeyRaw(), r.cfg.Message())
	c := sample.Scalar(cHash.Digest(), r.Group())

	// 4. Compute zᵢ = dᵢ + (eᵢ ρᵢ) + λᵢ sᵢ c
	ek, err := r.sign_e.GetKey(opts)
	if err != nil {
		return r, err
	}
	dk, err := r.sign_d.GetKey(opts)
	if err != nil {
		return r, err
	}
	ed := ek.Mul(rho[r.SelfID()])
	edk := r.sign_e.NewKey(ed, ed.ActOnBase(), r.Group())
	ed = dk.AddKeys(edk)

	signKey, err := r.ec_sign_km.GetKey(opts)
	if err != nil {
		return r, err
	}
	z := signKey.Commit(c, ed)

	// 5. Broadcast z
	if err := r.BroadcastMessage(out, &broadcast3{Z: z}); err != nil {
		return r, err
	}

	return nil, nil
}

func (r *round2) CanFinalize() bool {
	return true
}

// MessageContent implements round.Round.
func (round2) MessageContent() round.Content { return nil }

// RoundNumber implements round.Content.
func (broadcast2) RoundNumber() round.Number { return 2 }

// BroadcastContent implements round.BroadcastRound.
func (r *round2) BroadcastContent() round.BroadcastContent {
	return &broadcast2{
		D: r.Group().NewPoint(),
		E: r.Group().NewPoint(),
	}
}

// Number implements round.Round.
func (round2) Number() round.Number { return 2 }
