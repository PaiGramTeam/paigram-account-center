from __future__ import annotations


class AccountSDKError(Exception):
    """Base error raised by the SDK."""


class InvalidRequestError(AccountSDKError):
    """The request violates a public API or remote contract."""


class AuthenticationError(AccountSDKError):
    """The configured client or token could not be authenticated."""


class AuthorizationError(AccountSDKError):
    """The authenticated consumer lacks permission for the operation."""


class NotFoundError(AccountSDKError):
    """The requested user, binding, or platform resource does not exist."""


class ConflictError(AccountSDKError):
    """The request conflicts with the current remote state."""


class CredentialError(AccountSDKError):
    """Credential or binding state prevents the requested operation."""


class DeadlineExceededError(AccountSDKError):
    """The remote operation exceeded the configured timeout."""


class RateLimitError(AccountSDKError):
    """The remote service rejected the request because a rate limit was exceeded."""


class ServiceUnavailableError(AccountSDKError):
    """The remote service is temporarily unavailable."""


class TransportError(AccountSDKError):
    """The remote service could not be reached or returned an invalid response."""
