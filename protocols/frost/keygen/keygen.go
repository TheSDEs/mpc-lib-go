package keygen

import (
	"fmt"

	"github.com/mr-shifu/mpc-lib/core/pool"
	"github.com/mr-shifu/mpc-lib/core/protocol"
	"github.com/mr-shifu/mpc-lib/lib/round"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/commitment"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/ecdsa"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/hash"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/rid"
	"github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/vss"
	"github.com/mr-shifu/mpc-lib/pkg/keyopts"
	mpc_config "github.com/mr-shifu/mpc-lib/pkg/mpc/common/config"
	"github.com/mr-shifu/mpc-lib/pkg/mpc/common/message"
	mpc_state "github.com/mr-shifu/mpc-lib/pkg/mpc/common/state"
)

const (
	Rounds                    round.Number = 3
	KEYGEN_THRESHOLD_PROTOCOL string       = "frost/keygen-threshold"
)

type FROSTKeygen struct {
	configmgr   mpc_config.KeyConfigManager
	statemgr    mpc_state.MPCStateManager
	msgmgr      message.MessageManager
	bcstmgr     message.MessageManager
	ecdsa_km    ecdsa.ECDSAKeyManager
	ec_vss_km   ecdsa.ECDSAKeyManager
	vss_mgr     vss.VssKeyManager
	chainKey_km rid.RIDManager
	hash_mgr    hash.HashManager
	commit_mgr  commitment.CommitmentManager
}

func NewFROSTKeygen(
	keyconfigmgr mpc_config.KeyConfigManager,
	keystatmgr mpc_state.MPCStateManager,
	msgmgr message.MessageManager,
	bcstmgr message.MessageManager,
	ecdsa ecdsa.ECDSAKeyManager,
	ec_vss_km ecdsa.ECDSAKeyManager,
	vss_mgr vss.VssKeyManager,
	chainKey rid.RIDManager,
	hash_mgr hash.HashManager,
	commit_mgr commitment.CommitmentManager,
	pl *pool.Pool,
) *FROSTKeygen {
	return &FROSTKeygen{
		configmgr:   keyconfigmgr,
		statemgr:    keystatmgr,
		msgmgr:      msgmgr,
		bcstmgr:     bcstmgr,
		ecdsa_km:    ecdsa,
		ec_vss_km:   ec_vss_km,
		vss_mgr:     vss_mgr,
		chainKey_km: chainKey,
		hash_mgr:    hash_mgr,
		commit_mgr:  commit_mgr,
	}
}

func (m *FROSTKeygen) Start(cfg mpc_config.KeyConfig, pl *pool.Pool) protocol.StartFunc {
	return func(sessionID []byte) (_ round.Session, err error) {
		// TODO we should supprt taproot for next version
		info := round.Info{
			ProtocolID:       KEYGEN_THRESHOLD_PROTOCOL,
			SelfID:           cfg.SelfID(),
			PartyIDs:         cfg.PartyIDs(),
			Threshold:        cfg.Threshold(),
			Group:            cfg.Group(),
			FinalRoundNumber: Rounds,
		}

		// instantiate a new hasher for new keygen session
		opts := keyopts.Options{}
		opts.Set("id", cfg.ID(), "partyid", string(info.SelfID))
		h := m.hash_mgr.NewHasher(cfg.ID(), opts)

		// generate new helper for new keygen session
		_, err = round.NewSession(cfg.ID(), info, sessionID, pl, h)
		if err != nil {
			return nil, fmt.Errorf("keygen: %w", err)
		}

		return nil, nil
	}
}
