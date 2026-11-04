import { describe, expect, it } from "vitest";
import { auditEventDetail } from "./auditDetails";
import type { AuditEvent } from "./types";

function event(eventType: string, metadata: Record<string, unknown>): AuditEvent {
  return { id: "e1", eventType, userId: "u1", userName: "User One", metadata, createdAt: "2026-11-04T10:00:00Z" };
}

describe("auditEventDetail", () => {
  it("names the version and the before/after of a state change", () => {
    const detail = auditEventDetail(
      event("keyvault_secret_state_updated", {
        version: "1509b261163540c68bf40073236c31c6",
        enabled: false,
        notBefore: null,
        expiresAt: null,
        previous: { enabled: true, notBefore: null, expiresAt: null }
      })
    );
    expect(detail).toBe("version 1509b261 · disabled (was enabled) · from — · until —");
  });

  it("shows a changed date with its previous value", () => {
    const detail = auditEventDetail(
      event("keyvault_secret_state_updated", {
        version: "abc",
        enabled: true,
        expiresAt: "2027-01-01T00:00:00Z",
        previous: { enabled: true, expiresAt: null }
      })
    );
    expect(detail).toContain("enabled · from —");
    expect(detail).toMatch(/until .+ \(was —\)$/);
  });

  it("describes a written version without any value", () => {
    expect(
      auditEventDetail(event("keyvault_secret_value_set", { vault: "kvinsio", version: "9b5ad1d4dfea", inputMode: "file", fileName: "cert.pfx", enabled: true }))
    ).toBe("vault kvinsio · version 9b5ad1d4 · file cert.pfx · enabled");
  });

  it("names the version of a version reveal and stays empty for other events", () => {
    expect(auditEventDetail(event("resource_revealed", { type: "key_vault_secret", version: "a69cee0be38f" }))).toBe("version a69cee0b");
    expect(auditEventDetail(event("resource_revealed", { type: "web_portal" }))).toBe("");
    expect(auditEventDetail(event("login_succeeded", {}))).toBe("");
  });
});
