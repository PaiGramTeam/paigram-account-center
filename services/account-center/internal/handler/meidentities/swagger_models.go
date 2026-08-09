package meidentities

// swaggerListResponse is a Swagger-only model documenting the envelope
// wrapping the List endpoint's bot identities array. Its shape matches
// internal/response.Response specialised with []BotIdentityDTO data.
type swaggerListResponse struct {
	Code    int              `json:"code" example:"200"`
	Data    []BotIdentityDTO `json:"data"`
	Message string           `json:"message" example:"success"`
}

// swaggerErrorResponse documents the envelope used by List/Unlink failures.
// Mirrors internal/response.Response with a nil Data field.
type swaggerErrorResponse struct {
	Code    int    `json:"code" example:"404"`
	Data    any    `json:"data"`
	Message string `json:"message" example:"not_found"`
}
