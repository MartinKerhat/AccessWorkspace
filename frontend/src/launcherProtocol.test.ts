import { describe, expect, it } from "vitest";
import { browserBlocksLauncherBridge, buildLauncherProtocolUri } from "./launcherProtocol";
import type { LaunchPayload } from "./types";

const safariMac =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15";
const chromeMac =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const edgeWindows =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0";
const firefoxMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:143.0) Gecko/20100101 Firefox/143.0";
const safariIphone =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1";
const epiphanyLinux =
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15 Epiphany/605.1.15";

describe("browserBlocksLauncherBridge", () => {
  it("is true for Safari on a Mac", () => {
    expect(browserBlocksLauncherBridge({ userAgent: safariMac, platform: "MacIntel", maxTouchPoints: 0 })).toBe(true);
  });

  it("is true for other desktop WebKit browsers", () => {
    expect(browserBlocksLauncherBridge({ userAgent: epiphanyLinux, platform: "Linux x86_64", maxTouchPoints: 0 })).toBe(true);
  });

  it("is false for Chromium browsers, which also say Safari", () => {
    expect(browserBlocksLauncherBridge({ userAgent: chromeMac, platform: "MacIntel", maxTouchPoints: 0 })).toBe(false);
    expect(browserBlocksLauncherBridge({ userAgent: edgeWindows, platform: "Win32", maxTouchPoints: 0 })).toBe(false);
  });

  it("is false for Firefox", () => {
    expect(browserBlocksLauncherBridge({ userAgent: firefoxMac, platform: "MacIntel", maxTouchPoints: 0 })).toBe(false);
  });

  it("is false on iPhone and on an iPad posing as a Mac — no launcher exists there", () => {
    expect(browserBlocksLauncherBridge({ userAgent: safariIphone, platform: "iPhone", maxTouchPoints: 5 })).toBe(false);
    expect(browserBlocksLauncherBridge({ userAgent: safariMac, platform: "MacIntel", maxTouchPoints: 5 })).toBe(false);
  });
});

describe("buildLauncherProtocolUri", () => {
  const launch: LaunchPayload = {
    resourceId: "res-1",
    resourceType: "rdp",
    method: "launcher_ticket",
    target: "host.example",
    metadata: { launcherTicket: "ticket-123", username: "someone", port: "3389" }
  };
  const resolveUrl = "https://workspace.example/api/launcher/tickets/ticket-123";

  function decode(uri: string) {
    const encoded = new URL(uri).searchParams.get("payload") ?? "";
    const base64 = encoded.replace(/-/g, "+").replace(/_/g, "/");
    const binary = atob(base64 + "=".repeat((4 - (base64.length % 4)) % 4));
    return JSON.parse(new TextDecoder().decode(Uint8Array.from(binary, (char) => char.charCodeAt(0))));
  }

  it("builds a launch link the launcher can decode", () => {
    const uri = buildLauncherProtocolUri(launch, resolveUrl);
    expect(uri).toMatch(/^access-workspace:\/\/launch\?payload=[A-Za-z0-9_-]+$/);
    expect(decode(uri!)).toEqual({
      resourceId: "res-1",
      resourceType: "rdp",
      method: "launcher_ticket",
      target: "host.example",
      metadata: { launcherTicket: "ticket-123", launcherResolveUrl: resolveUrl }
    });
  });

  it("carries nothing but the ticket from the launch metadata", () => {
    const uri = buildLauncherProtocolUri({ ...launch, metadata: { ...launch.metadata, secretValue: "s3cret" } }, resolveUrl);
    const decoded = decode(uri!);
    expect(Object.keys(decoded.metadata).sort()).toEqual(["launcherResolveUrl", "launcherTicket"]);
    expect(JSON.stringify(decoded)).not.toContain("s3cret");
  });

  it("survives non-ASCII targets", () => {
    const uri = buildLauncherProtocolUri({ ...launch, target: "počítač.example" }, resolveUrl);
    expect(decode(uri!).target).toBe("počítač.example");
  });

  it("returns null without a ticket", () => {
    expect(buildLauncherProtocolUri({ ...launch, metadata: {} }, resolveUrl)).toBeNull();
  });
});
