import { describe, expect, it } from "vitest";
import { ownsResource } from "./resourceMeta";

const shared = { ownerUserId: "alice", ownerTeam: "ops", personal: false };

describe("ownsResource", () => {
  it("treats the owner user as owner", () => {
    expect(ownsResource(shared, { id: "alice", localGroups: [] })).toBe(true);
  });

  it("treats owner-team members as owners on shared objects", () => {
    expect(ownsResource(shared, { id: "bob", localGroups: ["ops"] })).toBe(true);
  });

  it("does not treat other groups as owners", () => {
    expect(ownsResource(shared, { id: "carol", localGroups: ["support"] })).toBe(false);
  });

  it("ignores the owner team on personal objects", () => {
    expect(ownsResource({ ...shared, personal: true }, { id: "bob", localGroups: ["ops"] })).toBe(false);
  });

  it("does not match an empty owner team", () => {
    expect(ownsResource({ ...shared, ownerTeam: "" }, { id: "bob", localGroups: [""] })).toBe(false);
  });
});
