package biz

type ServiceTicketClaims struct {
	TicketType       string
	ActorType        string
	ActorID          string
	OwnerUserRef     string
	EntryIdentityRef string
	// BindingRef is the first-class control-plane binding identity carried by
	// service tickets and used for authorization and resource lookup.
	BindingRef           string
	Platform             string
	AccountKey           string
	Consumer             string
	GrantVersion         uint64
	OwnerEpoch           uint64
	ConsumerEpoch        uint64
	EntryEpoch           uint64
	CredentialGeneration uint64
	OperationID          string
	ProfileRef           string
	Scopes               []string
	Audience             string
	BotID                string
	PlatformServiceKey   string
}
