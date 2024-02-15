package keystore

type Keystore interface {
	Import(keyID string, key []byte) error
	Get(keyID string) ([]byte, error)
	Delete(keyID string) error
}