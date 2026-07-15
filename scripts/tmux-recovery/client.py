#!/usr/bin/env python3
"""Thin CHROTE HTTP client for tmux recovery operator tools."""

from __future__ import annotations

from dataclasses import dataclass, field
import ipaddress
import json
from typing import Any
from urllib import error, parse, request


class ChroteClientError(RuntimeError):
    pass


class _NoRedirect(request.HTTPRedirectHandler):
    def redirect_request(self, req: Any, fp: Any, code: int, msg: str, headers: Any, newurl: str) -> None:
        return None


@dataclass
class ChroteClient:
    base_url: str
    token: str | None = None
    timeout: float = 10.0
    opener: Any | None = None
    _authorization_allowed: bool = field(init=False, repr=False)

    def __post_init__(self) -> None:
        self.base_url = normalize_api_base_url(self.base_url)
        self._authorization_allowed = _origin_allows_authorization(self.base_url)
        if self.opener is None:
            self.opener = request.build_opener(_NoRedirect)

    def get_sessions(self) -> dict[str, Any]:
        return self._request("GET", "/api/tmux/sessions")

    def update_session_recovery(self, name: str, unix_user: str, recovery_plan: list[dict[str, Any]]) -> dict[str, Any]:
        return self.update_session_recovery_entry(name, unix_user, {"recoveryPlan": recovery_plan})

    def update_session_recovery_entry(self, name: str, unix_user: str, body: dict[str, Any]) -> dict[str, Any]:
        query = _unix_user_query(unix_user)
        return self._request(
            "POST",
            f"/api/tmux/session-bank/{parse.quote(name, safe='')}/recovery{query}",
            body,
        )

    def forget_session_bank(self, name: str, unix_user: str) -> dict[str, Any]:
        query = _unix_user_query(unix_user)
        return self._request("DELETE", f"/api/tmux/session-bank/{parse.quote(name, safe='')}{query}")

    def restore_session_bank_entry(self, name: str, unix_user: str, entry: dict[str, Any]) -> dict[str, Any]:
        query = _unix_user_query(unix_user)
        return self._request(
            "PUT",
            f"/api/tmux/session-bank/{parse.quote(name, safe='')}/entry{query}",
            entry,
        )

    def recover_session(self, name: str, unix_user: str, body: dict[str, Any]) -> dict[str, Any]:
        query = _unix_user_query(unix_user)
        return self._request(
            "POST",
            f"/api/tmux/session-bank/{parse.quote(name, safe='')}/recover{query}",
            body,
        )

    def _request(self, method: str, path: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body, separators=(",", ":")).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.token and self._authorization_allowed:
            headers["Authorization"] = "Bearer " + self.token
        req = request.Request(self.base_url + path, data=data, headers=headers, method=method)
        try:
            with self.opener.open(req, timeout=self.timeout) as resp:
                raw = resp.read()
                if not raw:
                    return {}
                return json.loads(raw.decode("utf-8"))
        except error.HTTPError as exc:
            raw = exc.read().decode("utf-8", errors="replace")
            raise ChroteClientError(f"CHROTE API {method} {path} failed with HTTP {exc.code}: {_safe_body(raw)}") from exc
        except error.URLError as exc:
            raise ChroteClientError(f"CHROTE API {method} {path} failed: {exc.reason}") from exc
        except json.JSONDecodeError as exc:
            raise ChroteClientError(f"CHROTE API {method} {path} returned invalid JSON") from exc


def _unix_user_query(unix_user: str) -> str:
    unix_user = str(unix_user or "").strip()
    if not unix_user:
        return ""
    return "?" + parse.urlencode({"unixUser": unix_user})


def normalize_api_base_url(value: str) -> str:
    raw = str(value or "").strip()
    parsed = parse.urlsplit(raw)
    scheme = parsed.scheme.lower()
    if scheme not in {"http", "https"}:
        raise ChroteClientError("CHROTE API URL scheme must be http or https")
    if parsed.username or parsed.password:
        raise ChroteClientError("CHROTE API URL must not contain userinfo")
    if parsed.query or parsed.fragment:
        raise ChroteClientError("CHROTE API URL must not contain query or fragment")
    if parsed.path not in {"", "/"}:
        raise ChroteClientError("CHROTE API URL must be an origin with no path")
    host = parsed.hostname
    if not host:
        raise ChroteClientError("CHROTE API URL host is required")
    try:
        port = parsed.port
    except ValueError as exc:
        raise ChroteClientError("CHROTE API URL port is invalid") from exc
    host = host.lower()
    loopback = _is_loopback_host(host)
    if scheme == "http" and not loopback:
        raise ChroteClientError("CHROTE API URL must use https for non-loopback hosts")
    host_display = f"[{host}]" if ":" in host and not host.startswith("[") else host
    port_text = f":{port}" if port is not None else ""
    return f"{scheme}://{host_display}{port_text}"


def _origin_allows_authorization(origin: str) -> bool:
    parsed = parse.urlsplit(origin)
    return parsed.scheme == "https" or _is_loopback_host(str(parsed.hostname or ""))


def _is_loopback_host(host: str) -> bool:
    host = str(host or "").strip().lower()
    if host == "localhost":
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def _safe_body(raw: str) -> str:
    text = " ".join(str(raw or "").split())
    if len(text) > 300:
        text = text[:300] + "..."
    return text
