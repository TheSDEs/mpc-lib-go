package ecdsa

import (
	"crypto/sha256"
	"errors"

	"github.com/mr-shifu/mpc-lib/core/math/curve"
	comm_ecdsa "github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/ecdsa"
	comm_vss "github.com/mr-shifu/mpc-lib/pkg/common/cryptosuite/vss"
	zksch "github.com/mr-shifu/mpc-lib/pkg/cryptosuite/sw/zk-schnorr"
)

var (
	ErrInvalidKey = errors.New("invalid key")
)

type ECDSAKey struct {
	// Private key
	priv curve.Scalar

	// Public key
	pub curve.Point

	// group
	group curve.Curve

	zks *zksch.ZKSchnorr

	vssmgr comm_vss.VssKeyManager
}

func NewECDSAKey(priv curve.Scalar, pub curve.Point, group curve.Curve) ECDSAKey {
	return ECDSAKey{
		priv:  priv,
		pub:   pub,
		group: group,
	}
}

func (key ECDSAKey) Bytes() ([]byte, error) {
	pk, err := key.pub.MarshalBinary()
	if err != nil {
		return nil, err
	}
	gn := key.group.Name()

	buf := make([]byte, 0)
	buf = append(buf, uint8(len(gn)))
	buf = append(buf, []byte(gn)...)
	buf = append(buf, uint8(len(pk)))
	buf = append(buf, pk...)

	if key.Private() {
		sk, err := key.priv.MarshalBinary()
		if err != nil {
			return nil, err
		}
		buf = append(buf, uint8(len(sk)))
		buf = append(buf, sk...)
	}

	return buf, nil
}

func (key ECDSAKey) SKI() []byte {
	raw, err := key.pub.MarshalBinary()
	if err != nil {
		return nil
	}
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

func (key ECDSAKey) Private() bool {
	return key.priv != nil
}

func (key ECDSAKey) PublicKey() comm_ecdsa.ECDSAKey {
	return NewECDSAKey(nil, key.pub, key.group)
}

func (key ECDSAKey) Group() curve.Curve {
	return key.group
}

func (key ECDSAKey) PublicKeyRaw() curve.Point {
	return key.pub
}

func (key ECDSAKey) withZKSchnorr(zks *zksch.ZKSchnorr) ECDSAKey {
	key.zks = zks
	return key
}

func (key ECDSAKey) withVSSKeyMgr(vssmgr comm_vss.VssKeyManager) ECDSAKey {
	key.vssmgr = vssmgr
	return key
}

func fromBytes(data []byte) (ECDSAKey, error) {
	if len(data) < 2 {
		return ECDSAKey{}, ErrInvalidKey
	}

	gnLen := int(data[0])
	if len(data) < 2+gnLen {
		return ECDSAKey{}, ErrInvalidKey
	}
	gn := string(data[1 : 1+gnLen])
	var group curve.Curve
	switch gn {
	case "secp256k1":
		group = curve.Secp256k1{}
	}

	pkLen := int(data[1+gnLen])
	if len(data) < 2+gnLen+pkLen {
		return ECDSAKey{}, ErrInvalidKey
	}
	pk := group.NewPoint()
	if err := pk.UnmarshalBinary(data[2+gnLen : 2+gnLen+pkLen]); err != nil {
		return ECDSAKey{}, err
	}

	skLen := int(data[2+gnLen+pkLen])
	if len(data) < 2+gnLen+pkLen+skLen {
		return ECDSAKey{}, ErrInvalidKey
	}
	sk := group.NewScalar()
	if err := sk.UnmarshalBinary(data[3+gnLen+pkLen:]); err != nil {
		return ECDSAKey{}, err
	}

	return NewECDSAKey(sk, pk, group), nil
}
