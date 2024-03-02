package ecdsa

import (
	comm_ecdsa "github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/ecdsa"
)

type ECDSAKeyManager interface {
	GenerateKey(keyID string, partyID string) (comm_ecdsa.ECDSAKey, error)
	ImportKey(keyID string, partyID string, key comm_ecdsa.ECDSAKey) error
	GetKey(keyID string, partyID string) (comm_ecdsa.ECDSAKey, error)
}
