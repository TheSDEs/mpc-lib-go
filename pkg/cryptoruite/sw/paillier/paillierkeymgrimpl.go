package paillier

import (
	"encoding/hex"

	"github.com/cronokirby/saferith"
	"github.com/mr-shifu/mpc-lib/pkg/common/keystore"

	pailliercore "github.com/mr-shifu/mpc-lib/core/paillier"
	"github.com/mr-shifu/mpc-lib/core/pool"
)

type PaillierKeyManager struct {
	pl       *pool.Pool
	keystore keystore.Keystore
}

func NewPaillierKeyManager(store keystore.Keystore, pl *pool.Pool) *PaillierKeyManager {
	return &PaillierKeyManager{
		keystore: store,
	}
}

// GenerateKey generates a new Paillier key pair.
func (mgr *PaillierKeyManager) GenerateKey() (PaillierKey, error) {
	// generate a new Paillier key pair
	pk, sk := pailliercore.KeyGen(mgr.pl)
	key := PaillierKey{sk, pk}

	// get binary encoded of secret key params (P, Q)
	encoded, err := key.Bytes()
	if err != nil {
		return PaillierKey{}, err
	}

	// derive SKI from N param of public key
	ski := key.SKI()
	keyID := hex.EncodeToString(ski)

	// store the key to the keystore with keyID
	if err := mgr.keystore.Import(keyID, encoded); err != nil {
		return PaillierKey{}, err
	}

	return key, nil
}

// GetKey returns a Paillier key by its SKI.
func (mgr *PaillierKeyManager) GetKey(ski []byte) (PaillierKey, error) {
	// get the key from the keystore
	keyID := hex.EncodeToString(ski)
	decoded, err := mgr.keystore.Get(keyID)
	if err != nil {
		return PaillierKey{}, nil
	}

	// decode the key from the keystore
	key, err := fromBytes(decoded)
	if err != nil {
		return PaillierKey{}, nil
	}

	return key, nil
}

// ImportKey imports a Paillier key from its byte representation.
func (mgr *PaillierKeyManager) ImportKey(key PaillierKey) (error) {
	// encode the key into binary
	kb, err := key.Bytes()
	if err != nil {
		return err
	}

	// get SKI from key
	ski := key.SKI()
	keyID := hex.EncodeToString(ski)

	// store the key to the keystore with keyID
	return mgr.keystore.Import(keyID, kb)
}

// Encrypt returns the encryption of `message` as ciphertext and nonce generated by function.
func (mgr *PaillierKeyManager) Encode(ski []byte, m *saferith.Int) (*pailliercore.Ciphertext, *saferith.Nat) {
	key, err := mgr.GetKey(ski)
	if err != nil {
		return nil, nil
	}

	return key.publicKey.Enc(m)
}

// EncryptWithNonce returns the encryption of `message` as ciphertext and nonce passed to function.
func (mgr *PaillierKeyManager) EncWithNonce(ski []byte, m *saferith.Int, nonce *saferith.Nat) *pailliercore.Ciphertext {
	key, err := mgr.GetKey(ski)
	if err != nil {
		return nil
	}

	return key.publicKey.EncWithNonce(m, nonce)
}

// Decrypt returns the decryption of `ct` as ciphertext.
func (mgr *PaillierKeyManager) Decode(ski []byte, ct *pailliercore.Ciphertext) (*saferith.Int, error) {
	key, err := mgr.GetKey(ski)
	if err != nil {
		return nil, err
	}

	return key.secretKey.Dec(ct)
}

// DecryptWithNonce returns the decryption of `ct` as ciphertext and nonce.
func (mgr *PaillierKeyManager) DecodeWithNonce(ski []byte, ct *pailliercore.Ciphertext) (*saferith.Int, *saferith.Nat, error) {
	key, err := mgr.GetKey(ski)
	if err != nil {
		return nil, nil, err
	}

	return key.secretKey.DecWithRandomness(ct)
}

// ValidateCiphertexts returns true if all ciphertexts are valid.
func (mgr *PaillierKeyManager) ValidateCiphertexts(ski []byte, cts ...*pailliercore.Ciphertext) (bool, error) {
	key, err := mgr.GetKey(ski)
	if err != nil {
		return false, err
	}

	return key.secretKey.ValidateCiphertexts(cts...), nil
}
