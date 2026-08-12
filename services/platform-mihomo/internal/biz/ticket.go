package biz

type ServiceTicketClaims struct {
	TicketType  string
	ActorType   string
	ActorID     string
	OwnerUserID uint64
	// BindingID is the first-class control-plane binding identity carried by
	// service tickets and used for authorization and resource lookup.
	BindingID          uint64
	Platform           string
	PlatformAccountID  string
	Consumer           string
	GrantVersion       uint64
	ProfileID          uint64
	Scopes             []string
	Audience           string
	BotID              string
	UserID             uint64
	PlatformServiceKey string
}
