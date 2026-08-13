from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass

import httpx

from .errors import (
    AuthenticationError,
    AuthorizationError,
    DeadlineExceededError,
    InvalidRequestError,
    RateLimitError,
    ServiceUnavailableError,
    TransportError,
)

logger = logging.getLogger(__name__)

_DEFAULT_SCOPES = ("bot.access.read", "bot.access.issue_ticket")


@dataclass(frozen=True, slots=True)
class _AccessToken:
    value: str
    expires_at: float


class _ClientCredentialsTokenProvider:
    def __init__(
        self,
        http_client: httpx.AsyncClient,
        client_id: str,
        client_secret: str,
        timeout: float,
    ) -> None:
        self._http_client = http_client
        self._client_id = client_id
        self._client_secret = client_secret
        self._timeout = timeout
        self._cached: dict[tuple[str, ...], _AccessToken] = {}
        self._lock = asyncio.Lock()

    async def get(self, request_id: str, scopes: tuple[str, ...] = _DEFAULT_SCOPES) -> str:
        normalized_scopes = tuple(dict.fromkeys(scopes))
        cached = self._cached.get(normalized_scopes)
        if cached is not None and cached.expires_at > time.monotonic():
            return cached.value

        async with self._lock:
            cached = self._cached.get(normalized_scopes)
            if cached is not None and cached.expires_at > time.monotonic():
                return cached.value
            issued = await self._issue(request_id, normalized_scopes)
            self._cached[normalized_scopes] = issued
            return issued.value

    async def _issue(self, request_id: str, scopes: tuple[str, ...]) -> _AccessToken:
        try:
            response = await self._http_client.post(
                "/api/v1/oauth/token",
                data={
                    "grant_type": "client_credentials",
                    "client_id": self._client_id,
                    "client_secret": self._client_secret,
                    "audience": "account-center",
                    "scope": " ".join(scopes),
                },
                headers={"x-request-id": request_id},
                timeout=self._timeout,
            )
        except httpx.TimeoutException as error:
            logger.warning("Account Center token request timed out")
            raise DeadlineExceededError("Account Center token request timed out") from error
        except httpx.RequestError as error:
            logger.error("Account Center token request failed: %s", type(error).__name__)
            raise TransportError("Account Center token request failed") from error

        if response.is_error:
            self._raise_oauth_error(response)

        try:
            payload = response.json()
            access_token = str(payload["access_token"])
            expires_in = int(payload["expires_in"])
        except (KeyError, TypeError, ValueError) as error:
            logger.error("Account Center returned an invalid token response")
            raise TransportError("Account Center returned an invalid token response") from error
        if not access_token or expires_in <= 0:
            logger.error("Account Center returned an unusable token")
            raise TransportError("Account Center returned an unusable token")

        refresh_margin = min(30, max(1, expires_in // 10))
        return _AccessToken(access_token, time.monotonic() + expires_in - refresh_margin)

    @staticmethod
    def _raise_oauth_error(response: httpx.Response) -> None:
        try:
            payload = response.json()
            code = str(payload.get("error", ""))
            description = str(payload.get("error_description", ""))
        except (TypeError, ValueError):
            code = ""
            description = ""
        message = description or code or f"Account Center token request failed with HTTP {response.status_code}"
        if response.status_code == 429:
            logger.warning("Account Center token endpoint rate limit was exceeded")
            raise RateLimitError(message)
        if response.status_code >= 500:
            logger.warning("Account Center token endpoint is unavailable with HTTP %s", response.status_code)
            raise ServiceUnavailableError(message)
        if code == "invalid_client" or response.status_code == 401:
            logger.warning("Account Center rejected the configured client credentials")
            raise AuthenticationError(message)
        if code == "invalid_scope":
            logger.warning("Account Center rejected the requested SDK scopes")
            raise AuthorizationError(message)
        logger.warning("Account Center rejected the token request with HTTP %s", response.status_code)
        raise InvalidRequestError(message)
