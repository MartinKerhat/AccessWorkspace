#!/usr/bin/env python3
"""Mac App Store provisioning profiles for the Safari extension app, through
the App Store Connect API — no dashboard clicking, no profile secrets.

Used by scripts/build-safari-extension.sh --app-store. For each bundle id it
makes sure an ACTIVE "Mac App Store" profile exists that includes the given
"Apple Distribution" certificate (creating or replacing it as needed), installs
it where Xcode looks for profiles, and prints "<bundle id>\\t<profile name>"
lines for the export options.

Usage:
  asc-profiles.py ensure --cert-pem <Apple Distribution cert .pem> --bundle-id <id> [--bundle-id <id>...]

Environment (the same App Store Connect API key the notarization uses):
  NOTARY_KEY_PATH, NOTARY_KEY_ID, NOTARY_ISSUER_ID

Standard library only; the JWT is signed with the openssl command so this runs
on a bare macOS runner.
"""
import argparse
import base64
import json
import os
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

API = "https://api.appstoreconnect.apple.com"


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def der_ecdsa_to_raw(der: bytes) -> bytes:
    # SEQUENCE { INTEGER r, INTEGER s } -> r||s, 32 bytes each.
    assert der[0] == 0x30
    idx = 2 if der[1] < 0x80 else 2 + (der[1] & 0x7F)
    parts = []
    for _ in range(2):
        assert der[idx] == 0x02
        length = der[idx + 1]
        value = der[idx + 2 : idx + 2 + length]
        parts.append(value.lstrip(b"\x00").rjust(32, b"\x00"))
        idx += 2 + length
    return b"".join(parts)


def make_token() -> str:
    key_path, key_id, issuer = (os.environ.get(k, "") for k in ("NOTARY_KEY_PATH", "NOTARY_KEY_ID", "NOTARY_ISSUER_ID"))
    if not (key_path and key_id and issuer):
        sys.exit("NOTARY_KEY_PATH, NOTARY_KEY_ID and NOTARY_ISSUER_ID are required")
    now = int(time.time())
    header = b64url(json.dumps({"alg": "ES256", "kid": key_id, "typ": "JWT"}).encode())
    payload = b64url(json.dumps({"iss": issuer, "iat": now, "exp": now + 600, "aud": "appstoreconnect-v1"}).encode())
    signing_input = f"{header}.{payload}".encode()
    with tempfile.NamedTemporaryFile() as data:
        data.write(signing_input)
        data.flush()
        der = subprocess.run(["openssl", "dgst", "-sha256", "-sign", key_path, "-binary", data.name], check=True, capture_output=True).stdout
    return f"{header}.{payload}.{b64url(der_ecdsa_to_raw(der))}"


class Client:
    def __init__(self):
        self.token = make_token()

    def call(self, method: str, path: str, body=None):
        request = urllib.request.Request(API + path, method=method, headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"})
        data = json.dumps(body).encode() if body is not None else None
        try:
            with urllib.request.urlopen(request, data, timeout=60) as response:
                raw = response.read()
                return json.loads(raw) if raw else {}
        except urllib.error.HTTPError as error:
            detail = error.read().decode(errors="replace")
            sys.exit(f"App Store Connect API {method} {path} failed: {error.code}\n{detail}")


def find_certificate_id(client: Client, cert_pem: str) -> str:
    der = subprocess.run(["openssl", "x509", "-in", cert_pem, "-outform", "DER"], check=True, capture_output=True).stdout
    wanted = base64.b64encode(der).decode()
    page = client.call("GET", "/v1/certificates?limit=200")
    for item in page.get("data", []):
        if item["attributes"].get("certificateContent") == wanted:
            return item["id"]
    sys.exit(f"the certificate in {cert_pem} is not registered in App Store Connect")


def find_bundle_id(client: Client, identifier: str) -> str:
    page = client.call("GET", f"/v1/bundleIds?filter[identifier]={identifier}&limit=200")
    for item in page.get("data", []):
        if item["attributes"].get("identifier") == identifier:
            return item["id"]
    sys.exit(f"bundle id {identifier} is not registered under Certificates, Identifiers & Profiles")


def install_profile(profile: dict) -> None:
    content = base64.b64decode(profile["attributes"]["profileContent"])
    name = f'{profile["attributes"]["uuid"]}.provisionprofile'
    for directory in (
        os.path.expanduser("~/Library/Developer/Xcode/UserData/Provisioning Profiles"),  # Xcode 16+
        os.path.expanduser("~/Library/MobileDevice/Provisioning Profiles"),  # older Xcode
    ):
        os.makedirs(directory, exist_ok=True)
        with open(os.path.join(directory, name), "wb") as handle:
            handle.write(content)


def ensure_profile(client: Client, identifier: str, bundle_record_id: str, certificate_id: str, name: str) -> str:
    page = client.call("GET", f"/v1/profiles?filter[name]={urllib.request.quote(name)}&include=certificates&limit=10")
    for item in page.get("data", []):
        if item["attributes"].get("name") != name:
            continue
        certs = [c["id"] for c in item.get("relationships", {}).get("certificates", {}).get("data", [])]
        if item["attributes"].get("profileState") == "ACTIVE" and certificate_id in certs:
            install_profile(item)
            return name
        # Expired, invalidated by a certificate change, or built for another
        # certificate: replace it.
        client.call("DELETE", f'/v1/profiles/{item["id"]}')
    created = client.call("POST", "/v1/profiles", {
        "data": {
            "type": "profiles",
            "attributes": {"name": name, "profileType": "MAC_APP_STORE"},
            "relationships": {
                "bundleId": {"data": {"type": "bundleIds", "id": bundle_record_id}},
                "certificates": {"data": [{"type": "certificates", "id": certificate_id}]},
            },
        }
    })
    install_profile(created["data"])
    return name


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    ensure = sub.add_parser("ensure")
    ensure.add_argument("--cert-pem", required=True)
    ensure.add_argument("--bundle-id", action="append", required=True)
    args = parser.parse_args()

    client = Client()
    certificate_id = find_certificate_id(client, args.cert_pem)
    for identifier in args.bundle_id:
        bundle_record_id = find_bundle_id(client, identifier)
        name = f"{identifier} Mac App Store"
        ensure_profile(client, identifier, bundle_record_id, certificate_id, name)
        print(f"{identifier}\t{name}")


if __name__ == "__main__":
    main()
