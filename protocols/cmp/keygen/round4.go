package keygen

import (
	"errors"

	"github.com/mr-shifu/mpc-lib/core/math/curve"
	"github.com/mr-shifu/mpc-lib/core/math/polynomial"
	"github.com/mr-shifu/mpc-lib/core/paillier"
	"github.com/mr-shifu/mpc-lib/core/party"
	zkfac "github.com/mr-shifu/mpc-lib/core/zk/fac"
	zkmod "github.com/mr-shifu/mpc-lib/core/zk/mod"
	zkprm "github.com/mr-shifu/mpc-lib/core/zk/prm"
	"github.com/mr-shifu/mpc-lib/lib/round"
	comm_elgamal "github.com/mr-shifu/mpc-lib/pkg/mpc/common/elgamal"
	comm_mpc_ks "github.com/mr-shifu/mpc-lib/pkg/mpc/common/mpckey"
	comm_paillier "github.com/mr-shifu/mpc-lib/pkg/mpc/common/paillier"
	comm_pedersen "github.com/mr-shifu/mpc-lib/pkg/mpc/common/pedersen"
	comm_rid "github.com/mr-shifu/mpc-lib/pkg/mpc/common/rid"
	comm_vss "github.com/mr-shifu/mpc-lib/pkg/mpc/common/vss"
	"github.com/mr-shifu/mpc-lib/protocols/cmp/config"
)

var _ round.Round = (*round4)(nil)

type round4 struct {
	*round3

	mpc_ks      comm_mpc_ks.MPCKeystore
	elgamal_km  comm_elgamal.ElgamalKeyManager
	paillier_km comm_paillier.PaillierKeyManager
	pedersen_km comm_pedersen.PedersenKeyManager
	vss_km      comm_vss.VssKeyManager
	rid_km      comm_rid.RIDKeyManager
	chainKey_km comm_rid.RIDKeyManager

	// Number of Broacasted Messages received
	MessageBroadcasted map[party.ID]bool
	MessagesForwarded  map[party.ID]bool
}

type message4 struct {
	// Share = Encᵢ(x) is the encryption of the receivers share
	Share *paillier.Ciphertext
	Fac   *zkfac.Proof
}

type broadcast4 struct {
	round.NormalBroadcastContent
	Mod *zkmod.Proof
	Prm *zkprm.Proof
}

// StoreBroadcastMessage implements round.BroadcastRound.
//
// - verify Mod, Prm proof for N
func (r *round4) StoreBroadcastMessage(msg round.Message) error {
	from := msg.From
	body, ok := msg.Content.(*broadcast4)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	// verify zkmod
	ped, err := r.pedersen_km.GetKey(r.KeyID, string(from))
	if err != nil {
		return err
	}
	paillier, err := r.paillier_km.GetKey(r.KeyID, string(from))
	if err != nil {
		return err
	}
	if !paillier.VerifyZKMod(body.Mod, r.HashForID(from), r.Pool) {
		return errors.New("failed to validate mod proof")
	}

	// verify zkprm
	if !ped.VerifyProof(r.HashForID(from), r.Pool, body.Prm) {
		return errors.New("failed to validate prm proof")
	}

	// Mark the message as received
	r.MessageBroadcasted[from] = true

	return nil
}

// VerifyMessage implements round.Round.
//
// - verify validity of share ciphertext.
func (r *round4) VerifyMessage(msg round.Message) error {
	from := msg.From
	body, ok := msg.Content.(*message4)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	paillierKey, err := r.paillier_km.GetKey(r.KeyID, string(r.SelfID()))
	if err != nil {
		return err
	}
	if !paillierKey.ValidateCiphertexts(body.Share) {
		return errors.New("invalid ciphertext")
	}

	ped, err := r.pedersen_km.GetKey(r.KeyID, string(r.SelfID()))
	if err != nil {
		return err
	}

	// verify zkfac
	paillierj, err := r.paillier_km.GetKey(r.KeyID, string(from))
	if err != nil {
		return err
	}
	if !paillierKey.VerifyZKFAC(body.Fac, zkfac.Public{
		N:   paillierj.PublicKey().ParamN(),
		Aux: ped.PublicKeyRaw(),
	}, r.HashForID(from)) {
		return errors.New("failed to validate fac proof")
	}

	return nil
}

// StoreMessage implements round.Round.
//
// Since this message is only intended for us, we need to do the VSS verification here.
// - check that the decrypted share did not overflow.
// - check VSS condition.
// - save share.
func (r *round4) StoreMessage(msg round.Message) error {
	from, body := msg.From, msg.Content.(*message4)

	// decrypt share
	paillierKey, err := r.paillier_km.GetKey(r.KeyID, string(r.SelfID()))
	if err != nil {
		return err
	}
	DecryptedShare, err := paillierKey.Decode(body.Share)
	if err != nil {
		return err
	}
	Share := r.Group().NewScalar().SetNat(DecryptedShare.Mod(r.Group().Order()))
	if DecryptedShare.Eq(curve.MakeInt(Share)) != 1 {
		return errors.New("decrypted share is not in correct range")
	}

	// verify share with VSS
	vssKey, err := r.vss_km.GetKey(r.KeyID, string(from))
	if err != nil {
		return err
	}
	ExpectedPublicShare, err := vssKey.EvaluateByExponents(r.SelfID().Scalar(r.Group())) // Fⱼ(i)
	if err != nil {
		return err
	}
	PublicShare := Share.ActOnBase()
	// X == Fⱼ(i)
	if !PublicShare.Equal(ExpectedPublicShare) {
		return errors.New("failed to validate VSS share")
	}

	if err := vssKey.ImportShare(r.SelfID().Scalar(r.Group()), Share); err != nil {
		return err
	}

	// Mark the message as received
	r.MessagesForwarded[from] = true

	return nil
}

// Finalize implements round.Round
//
// - sum of all received shares
// - compute group public key and individual public keys
// - recompute config SSID
// - validate Config
// - write new ssid hash to old hash state
// - create proof of knowledge of secret.
func (r *round4) Finalize(out chan<- *round.Message) (round.Session, error) {
	// check if we received all messages
	if len(r.MessageBroadcasted) != r.N()-1 || len(r.MessagesForwarded) != r.N()-1 {
		return nil, round.ErrNotEnoughMessages
	}
	// add all shares to our secret
	UpdatedSecretECDSA := r.Group().NewScalar()
	if r.PreviousSecretECDSA != nil {
		UpdatedSecretECDSA.Set(r.PreviousSecretECDSA)
	}
	for _, j := range r.PartyIDs() {
		vssKeyj, err := r.vss_km.GetKey(r.KeyID, string(j))
		if err != nil {
			return nil, err
		}
		shareReceivedj, err := vssKeyj.GetShare(r.SelfID().Scalar(r.Group()))
		if err != nil {
			return nil, err
		}
		UpdatedSecretECDSA.Add(shareReceivedj)
	}

	// [F₁(X), …, Fₙ(X)]
	ShamirPublicPolynomials := make([]*polynomial.Exponent, 0, len(r.PartyIDs()))
	for _, j := range r.PartyIDs() {
		vssj, err := r.vss_km.GetKey(r.KeyID, string(j))
		if err != nil {
			return r, err
		}
		expj, err := vssj.ExponentsRaw()
		if err != nil {
			return r, err
		}
		ShamirPublicPolynomials = append(ShamirPublicPolynomials, expj)
	}

	// ShamirPublicPolynomial = F(X) = ∑Fⱼ(X)
	ShamirPublicPolynomial, err := polynomial.Sum(ShamirPublicPolynomials)
	if err != nil {
		return r, err
	}

	// compute the new public key share Xⱼ = F(j) (+X'ⱼ if doing a refresh)
	PublicData := make(map[party.ID]*config.Public, len(r.PartyIDs()))
	for _, j := range r.PartyIDs() {
		PublicECDSAShare := ShamirPublicPolynomial.Evaluate(j.Scalar(r.Group()))
		if r.PreviousPublicSharesECDSA != nil {
			PublicECDSAShare = PublicECDSAShare.Add(r.PreviousPublicSharesECDSA[j])
		}
		elgamalj, err := r.elgamal_km.GetKey(r.KeyID, string(j))
		if err != nil {
			return r, err
		}

		paillierj, err := r.paillier_km.GetKey(r.KeyID, string(j))
		if err != nil {
			return r, err
		}

		pedersenj, err := r.pedersen_km.GetKey(r.KeyID, string(j))
		if err != nil {
			return r, err
		}

		PublicData[j] = &config.Public{
			ECDSA:    PublicECDSAShare,
			ElGamal:  elgamalj.PublicKeyRaw(),
			Paillier: paillierj.PublicKeyRaw(),
			Pedersen: pedersenj.PublicKeyRaw(),
		}
	}

	// rid, err := r.rid_km.GetKey(r.KeyID, string(r.SelfID()))
	// if err != nil {
	// 	return r, err
	// }

	// chainKey, err := r.chainKey_km.GetKey(r.KeyID, string(r.SelfID()))
	// if err != nil {
	// 	return r, err
	// }

	mpckey, err := r.mpc_ks.Get(r.KeyID)
	if err != nil {
		return r, err
	}

	// TODO elgamal and paillier secret key is missed here
	UpdatedConfig := &config.Config{
		Group:     r.Group(),
		ID:        r.SelfID(),
		Threshold: r.Threshold(),
		ECDSA:     UpdatedSecretECDSA,
		// ElGamal:   r.ElGamalSecret,
		// Paillier:  r.PaillierSecret,
		RID:      mpckey.RID,
		ChainKey: mpckey.ChainKey,
		Public:   PublicData,
	}

	// write new ssid to hash, to bind the Schnorr proof to this new config
	// Write SSID, selfID to temporary hash
	h := r.Hash().Clone()
	_ = h.WriteAny(UpdatedConfig, r.SelfID())

	proof := r.SchnorrRand.Prove(h, PublicData[r.SelfID()].ECDSA, UpdatedSecretECDSA, nil)

	// send to all
	err = r.BroadcastMessage(out, &broadcast5{SchnorrResponse: proof})
	if err != nil {
		return r, err
	}

	r.UpdateHashState(UpdatedConfig)
	return &round5{
		round4:             r,
		mpc_ks:             r.mpc_ks,
		UpdatedConfig:      UpdatedConfig,
		MessageBroadcasted: make(map[party.ID]bool),
	}, nil
}

func (r *round4) CanFinalize() bool {
	// Verify if all parties commitments are received
	return len(r.MessageBroadcasted) == r.N()-1 && len(r.MessagesForwarded) == r.N()-1
}

// RoundNumber implements round.Content.
func (message4) RoundNumber() round.Number { return 4 }

// MessageContent implements round.Round.
func (round4) MessageContent() round.Content { return &message4{} }

// RoundNumber implements round.Content.
func (broadcast4) RoundNumber() round.Number { return 4 }

// BroadcastContent implements round.BroadcastRound.
func (round4) BroadcastContent() round.BroadcastContent { return &broadcast4{} }

// Number implements round.Round.
func (round4) Number() round.Number { return 4 }
