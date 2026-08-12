package serviceticket

import contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"

const (
	TypeControl    = contractticket.TypeControl
	TypeDelegation = contractticket.TypeDelegation
)

var ErrInvalidConfig = contractticket.ErrInvalidIssuerConfig

type Config = contractticket.IssuerConfig
type Claims = contractticket.Claims
type Signer = contractticket.Issuer

func NewSigner(config Config) (*Signer, error) {
	return contractticket.NewIssuer(config)
}
