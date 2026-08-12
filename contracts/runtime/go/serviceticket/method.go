package serviceticket

import "github.com/golang-jwt/jwt/v5"

const (
	AlgorithmEd25519 = "Ed25519"
	TypeControl      = "paigram-platform-control+jwt"
	TypeDelegation   = "paigram-platform-delegation+jwt"
)

type signingMethodEd25519 struct {
	delegate *jwt.SigningMethodEd25519
}

func (m *signingMethodEd25519) Alg() string {
	return AlgorithmEd25519
}

func (m *signingMethodEd25519) Sign(signingString string, key any) ([]byte, error) {
	return m.delegate.Sign(signingString, key)
}

func (m *signingMethodEd25519) Verify(signingString string, signature []byte, key any) error {
	return m.delegate.Verify(signingString, signature, key)
}

var SigningMethodEd25519 jwt.SigningMethod = &signingMethodEd25519{delegate: jwt.SigningMethodEdDSA}

func init() {
	jwt.RegisterSigningMethod(AlgorithmEd25519, func() jwt.SigningMethod {
		return SigningMethodEd25519
	})
}
