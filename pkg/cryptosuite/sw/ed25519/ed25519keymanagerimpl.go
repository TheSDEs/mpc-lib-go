package ed25519

import (
	"encoding/hex"

	ed "filippo.io/edwards25519"
	"github.com/mr-shifu/mpc-lib/pkg/common/keystore"
	"github.com/mr-shifu/mpc-lib/pkg/keyopts"
	"github.com/pkg/errors"
)

type Ed25519KeyManagerImpl struct {
	keystore keystore.Keystore
}

func NewEd25519KeyManagerImpl(store keystore.Keystore) *Ed25519KeyManagerImpl {
	return &Ed25519KeyManagerImpl{
		keystore: store,
	}
}

func (mgr *Ed25519KeyManagerImpl) NewKey(priv *ed.Scalar, pub *ed.Point) (Ed25519, error) {
	if pub.Equal((&ed.Point{}).ScalarBaseMult(priv)) != 1 {
		return nil, errors.New("ed25519: public key does not match private key")
	}
	return &Ed25519Impl{
		s: priv,
		a: pub,
	}, nil
}

// GenerateKey generates a new Ed25519 key pair.
func (mgr *Ed25519KeyManagerImpl) GenerateKey(opts keyopts.Options) (Ed25519, error) {
	k, err := GenerateKey()
	if err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to generate key")
	}

	kb, err := k.Bytes()
	if err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to serialize key")
	}

	keyID := hex.EncodeToString(k.SKI())

	if err := mgr.keystore.Import(keyID, kb, opts); err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to import key to keystore")
	}

	return k, nil
}

// Import imports a Ed25519 key from its byte representation.
func (mgr *Ed25519KeyManagerImpl) ImportKey(raw interface{}, opts keyopts.Options) (Ed25519, error) {
	k := new(Ed25519Impl)
	switch tt := raw.(type) {
	case []byte:
		if err := k.FromBytes(tt); err != nil {
			return nil, errors.WithMessage(err, "ed25519: failed to import key")
		}
	case Ed25519:
		k = tt.(*Ed25519Impl)
	default:
		return nil, errors.New("ed25519: invalid key type")
	}

	kb, err := k.Bytes()
	if err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to serialize key")
	}

	keyID := hex.EncodeToString(k.SKI())

	if err := mgr.keystore.Import(keyID, kb, opts); err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to import key to keystore")
	}

	return k, nil
}

// GetKey returns a Ed25519 key by its SKI.
func (mgr *Ed25519KeyManagerImpl) GetKey(opts keyopts.Options) (Ed25519, error) {
	kb, err := mgr.keystore.Get(opts)
	if err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to get key from keystore")
	}

	k := new(Ed25519Impl)
	if err := k.FromBytes(kb); err != nil {
		return nil, errors.WithMessage(err, "ed25519: failed to import key")
	}

	return k, nil
}