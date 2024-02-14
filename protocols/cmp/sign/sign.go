package sign

import (
	"errors"
	"fmt"

	"github.com/mr-shifu/mpc-lib/lib/round"
	"github.com/mr-shifu/mpc-lib/lib/types"
	"github.com/mr-shifu/mpc-lib/pkg/math/curve"
	"github.com/mr-shifu/mpc-lib/pkg/math/polynomial"
	"github.com/mr-shifu/mpc-lib/pkg/paillier"
	"github.com/mr-shifu/mpc-lib/pkg/party"
	"github.com/mr-shifu/mpc-lib/pkg/pedersen"
	"github.com/mr-shifu/mpc-lib/pkg/pool"
	"github.com/mr-shifu/mpc-lib/pkg/protocol"
	"github.com/mr-shifu/mpc-lib/protocols/cmp/config"
)

// protocolSignID for the "3 round" variant using echo broadcast.
const (
	protocolSignID                  = "cmp/sign"
	protocolSignRounds round.Number = 5
)

func StartSign(config *config.Config, signers []party.ID, message []byte, pl *pool.Pool) protocol.StartFunc {
	return func(sessionID []byte) (round.Session, error) {
		group := config.Group

		// this could be used to indicate a pre-signature later on
		if len(message) == 0 {
			return nil, errors.New("sign.Create: message is nil")
		}

		info := round.Info{
			ProtocolID:       protocolSignID,
			FinalRoundNumber: protocolSignRounds,
			SelfID:           config.ID,
			PartyIDs:         signers,
			Threshold:        config.Threshold,
			Group:            config.Group,
		}

		helper, err := round.NewSession(info, sessionID, pl, config, types.SigningMessage(message))
		if err != nil {
			return nil, fmt.Errorf("sign.Create: %w", err)
		}

		if !config.CanSign(helper.PartyIDs()) {
			return nil, errors.New("sign.Create: signers is not a valid signing subset")
		}

		// Scale public data
		T := helper.N()
		ECDSA := make(map[party.ID]curve.Point, T)
		Paillier := make(map[party.ID]*paillier.PublicKey, T)
		Pedersen := make(map[party.ID]*pedersen.Parameters, T)
		PublicKey := group.NewPoint()
		lagrange := polynomial.Lagrange(group, signers)
		// Scale own secret
		SecretECDSA := group.NewScalar().Set(lagrange[config.ID]).Mul(config.ECDSA)
		SecretPaillier := config.Paillier
		for _, j := range helper.PartyIDs() {
			public := config.Public[j]
			// scale public key share
			ECDSA[j] = lagrange[j].Act(public.ECDSA)
			Paillier[j] = public.Paillier
			Pedersen[j] = public.Pedersen
			PublicKey = PublicKey.Add(ECDSA[j])
		}

		return &round1{
			Helper:         helper,
			PublicKey:      PublicKey,
			SecretECDSA:    SecretECDSA,
			SecretPaillier: SecretPaillier,
			Paillier:       Paillier,
			Pedersen:       Pedersen,
			ECDSA:          ECDSA,
			Message:        message,
		}, nil
	}
}
