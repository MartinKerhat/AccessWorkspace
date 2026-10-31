#!/usr/bin/env python3
"""Submit an uploaded Safari extension build for Mac App Store review through
the App Store Connect API, so a merged version bump goes all the way to
"Waiting for Review" without dashboard clicks.

Usage:
  asc-submit.py --bundle-id <id> --version <x.y.z> --build-number <n> [--whats-new <file>] [--dry-run]

Steps (each idempotent, so a re-run picks up where it stopped):
  1. wait until the uploaded build (CFBundleVersion = build number) is processed
  2. find the app's editable macOS version, or create one, and set its version string
  3. "What's New" from the file (first submission needs none); build attached;
     export compliance declared (no non-exempt encryption)
  4. App Review contact/notes copied from the previous version when the new one has none
  5. review submission created with the version and submitted (skipped with --dry-run)

Environment: NOTARY_KEY_PATH, NOTARY_KEY_ID, NOTARY_ISSUER_ID (App Store Connect API key).
Standard library only, like asc-profiles.py, whose API client this reuses.
"""
import argparse
import importlib.util
import os
import sys
import time

helper = importlib.util.spec_from_file_location("ascp", os.path.join(os.path.dirname(os.path.abspath(__file__)), "asc-profiles.py"))
ascp = importlib.util.module_from_spec(helper)
helper.loader.exec_module(ascp)

EDITABLE = {"PREPARE_FOR_SUBMISSION", "DEVELOPER_REJECTED", "REJECTED", "METADATA_REJECTED", "INVALID_BINARY"}


def wait_for_build(client, app_id, build_number, timeout_minutes=40):
    deadline = time.time() + timeout_minutes * 60
    while time.time() < deadline:
        builds = client.call("GET", f"/v1/builds?filter[app]={app_id}&filter[version]={build_number}&limit=1")["data"]
        if builds:
            state = builds[0]["attributes"]["processingState"]
            print(f"build {build_number}: {state}")
            if state == "VALID":
                return builds[0]
            if state in ("FAILED", "INVALID"):
                sys.exit(f"build {build_number} failed processing")
        else:
            print(f"build {build_number}: not listed yet")
        time.sleep(30)
    sys.exit(f"build {build_number} was not processed within {timeout_minutes} minutes")


def editable_version(client, app_id, version_string):
    versions = client.call("GET", f"/v1/apps/{app_id}/appStoreVersions?filter[platform]=MAC_OS&limit=20")["data"]
    for v in versions:
        if v["attributes"]["versionString"] == version_string:
            return v
    for v in versions:
        if v["attributes"]["appVersionState"] in EDITABLE:
            client.call("PATCH", f"/v1/appStoreVersions/{v['id']}", {"data": {"type": "appStoreVersions", "id": v["id"], "attributes": {"versionString": version_string}}})
            print(f"renamed editable version to {version_string}")
            return v
    created = client.call("POST", "/v1/appStoreVersions", {"data": {"type": "appStoreVersions", "attributes": {"platform": "MAC_OS", "versionString": version_string, "releaseType": "AFTER_APPROVAL"},
        "relationships": {"app": {"data": {"type": "apps", "id": app_id}}}}})["data"]
    print(f"created version {version_string}")
    return created


def previous_version(client, app_id, exclude_id):
    versions = client.call("GET", f"/v1/apps/{app_id}/appStoreVersions?filter[platform]=MAC_OS&limit=20")["data"]
    for v in versions:
        if v["id"] != exclude_id:
            return v
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--bundle-id", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--build-number", required=True)
    parser.add_argument("--whats-new", help="text file with the release notes shown on the store (required by Apple for updates)")
    parser.add_argument("--dry-run", action="store_true", help="prepare everything but do not submit for review")
    args = parser.parse_args()

    client = ascp.Client()
    apps = client.call("GET", f"/v1/apps?filter[bundleId]={args.bundle_id}")["data"]
    if not apps:
        sys.exit(f"no App Store Connect app with bundle id {args.bundle_id}")
    app_id = apps[0]["id"]

    build = wait_for_build(client, app_id, args.build_number)
    if build["attributes"].get("usesNonExemptEncryption") is None:
        client.call("PATCH", f"/v1/builds/{build['id']}", {"data": {"type": "builds", "id": build["id"], "attributes": {"usesNonExemptEncryption": False}}})

    version = editable_version(client, app_id, args.version)
    if version["attributes"]["appVersionState"] not in EDITABLE:
        sys.exit(f"version {args.version} is in state {version['attributes']['appVersionState']} and cannot be changed")
    client.call("PATCH", f"/v1/appStoreVersions/{version['id']}/relationships/build", {"data": {"type": "builds", "id": build["id"]}})
    print(f"attached build {args.build_number} to version {args.version}")

    whats_new = open(args.whats_new).read().strip() if args.whats_new and os.path.exists(args.whats_new) else ""
    if whats_new:
        for loc in client.call("GET", f"/v1/appStoreVersions/{version['id']}/appStoreVersionLocalizations")["data"]:
            try:
                client.call("PATCH", f"/v1/appStoreVersionLocalizations/{loc['id']}", {"data": {"type": "appStoreVersionLocalizations", "id": loc["id"], "attributes": {"whatsNew": whats_new}}})
                print(f"release notes set ({loc['attributes']['locale']})")
            except SystemExit as error:
                # Apple does not accept "What's New" on an app's first version.
                if "whatsNew" in str(error) and "cannot be edited" in str(error):
                    print("release notes not accepted on this version (first release); skipped")
                else:
                    raise

    detail = client.call("GET", f"/v1/appStoreVersions/{version['id']}/appStoreReviewDetail").get("data")
    if not detail:
        prev = previous_version(client, app_id, version["id"])
        prev_detail = client.call("GET", f"/v1/appStoreVersions/{prev['id']}/appStoreReviewDetail").get("data") if prev else None
        if prev_detail:
            attrs = {k: v for k, v in prev_detail["attributes"].items() if v is not None}
            client.call("POST", "/v1/appStoreReviewDetails", {"data": {"type": "appStoreReviewDetails", "attributes": attrs,
                "relationships": {"appStoreVersion": {"data": {"type": "appStoreVersions", "id": version["id"]}}}}})
            print("review contact and notes copied from the previous version")
        else:
            sys.exit("no App Review contact information on this or a previous version; fill it in App Store Connect once")

    if args.dry_run:
        print(f"dry run: version {args.version} with build {args.build_number} is ready; not submitted")
        return

    submission = client.call("POST", "/v1/reviewSubmissions", {"data": {"type": "reviewSubmissions", "attributes": {"platform": "MAC_OS"},
        "relationships": {"app": {"data": {"type": "apps", "id": app_id}}}}})["data"]
    client.call("POST", "/v1/reviewSubmissionItems", {"data": {"type": "reviewSubmissionItems",
        "relationships": {"reviewSubmission": {"data": {"type": "reviewSubmissions", "id": submission["id"]}},
                          "appStoreVersion": {"data": {"type": "appStoreVersions", "id": version["id"]}}}}})
    client.call("PATCH", f"/v1/reviewSubmissions/{submission['id']}", {"data": {"type": "reviewSubmissions", "id": submission["id"], "attributes": {"submitted": True}}})
    print(f"submitted version {args.version} (build {args.build_number}) for review")


if __name__ == "__main__":
    main()
