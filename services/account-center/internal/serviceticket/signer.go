package serviceticket

import (
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
)

const (
	TypeControl    = contractticket.TypeControl
	TypeDelegation = contractticket.TypeDelegation
)

var ErrInvalidConfig = contractticket.ErrInvalidIssuerConfig

type Config = contractticket.IssuerConfig
type Claims = contractticket.Claims
type Signer = contractticket.TicketIssuer

func NewSigner(config Config) (Signer, error) {
	return contractticket.NewIssuer(config)
}

func NewFileSigner(issuer string, ttl time.Duration, path string) (Signer, error) {
	return contractticket.NewFileIssuer(contractticket.FileIssuerConfig{
		Issuer: issuer, TTL: ttl, SigningKeyFile: path,
	})
}
