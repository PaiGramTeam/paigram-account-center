from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import Enum


@dataclass(frozen=True, slots=True)
class PlatformEndpoint:
    target: str
    secure: bool = True
    root_certificates: bytes | None = None


@dataclass(frozen=True, slots=True)
class BotUser:
    user_id: int
    bot_id: str
    external_user_id: str
    external_username: str


class PlatformAccountStatus(Enum):
    UNSPECIFIED = "unspecified"
    ACTIVE = "active"
    INACTIVE = "inactive"
    REVOKED = "revoked"


@dataclass(frozen=True, slots=True)
class PlatformBinding:
    binding_ref: str
    platform: str
    platform_service_key: str
    account_key: str
    display_name: str
    status: PlatformAccountStatus
    generation: int
    created_at: datetime | None
    updated_at: datetime | None


@dataclass(frozen=True, slots=True)
class ServiceTicket:
    token: str
    audience: str
    expires_at: datetime | None
    binding: PlatformBinding


@dataclass(frozen=True, slots=True)
class PlatformDescriptor:
    platform_key: str
    display_name: str
    service_audience: str
    supported_actions: tuple[str, ...]
    credential_schema: dict[str, object]
    contract_version: str


class CredentialStatus(Enum):
    UNSPECIFIED = "unspecified"
    ACTIVE = "active"
    EXPIRED = "expired"
    INVALID = "invalid"
    CHALLENGE_REQUIRED = "challenge_required"


@dataclass(frozen=True, slots=True)
class DeviceSummary:
    device_ref: str
    device_name: str
    is_valid: bool
    last_seen_at: datetime | None


@dataclass(frozen=True, slots=True)
class ProfileSummary:
    profile_ref: str
    account_key: str
    game_biz: str
    region: str
    player_id: str
    nickname: str
    level: int
    is_default: bool


@dataclass(frozen=True, slots=True)
class CredentialStatusResult:
    status: CredentialStatus
    last_validated_at: datetime | None


@dataclass(frozen=True, slots=True)
class ValidationResult:
    status: CredentialStatus
    reason_code: str


@dataclass(frozen=True, slots=True)
class AuthKey:
    value: str
    expires_at: datetime | None
