import test from "node:test";
import assert from "node:assert/strict";
import { deviceHost, splitTarget, joinTarget, groupRules, nextLocalPort, createRulesRefresher } from "./devices.js";

const mac = { id: "mac", name: "Mac mini", dnsName: "mac-mini.example.ts.net", addresses: ["100.64.0.1", "fd7a:115c:a1e0::1"] };

test("existing short DNS, full DNS and IPv6 mappings attach to the same device", () => {
  const rules = ["MAC-MINI:22", "mac-mini.example.ts.net.:5900", "[fd7a:115c:a1e0::1]:443", "100.64.0.1:80", "other:22"].map((target, i) => ({ id: String(i), target }));
  const grouped = groupRules([mac], rules);
  assert.equal(grouped.byDevice.get("mac").length, 4);
  assert.deepEqual(grouped.unmatched, [rules[4]]);
});

test("ambiguous short names never attach a rule to the wrong device", () => {
  const shared = { ...mac, id: "shared", dnsName: "mac-mini.other.ts.net", addresses: ["100.64.0.2"] };
  const rule = { id: "one", target: "mac-mini:22" };
  const grouped = groupRules([mac, shared], [rule]);
  assert.deepEqual(grouped.unmatched, [rule]);
});

test("device form supports DNS, IPv4 fallback and IPv6 without malformed targets", () => {
  assert.equal(deviceHost(mac), mac.dnsName);
  assert.equal(deviceHost({ addresses: ["::1", "100.64.0.3"] }), "100.64.0.3");
  assert.deepEqual(splitTarget("[fd7a:115c:a1e0::1]:5900"), { host: "fd7a:115c:a1e0::1", port: "5900" });
  assert.equal(joinTarget("fd7a:115c:a1e0::1", 22), "[fd7a:115c:a1e0::1]:22");
  assert.equal(joinTarget("host.example.", 22), "host.example.:22");
});

test("suggested local port skips saved rules", () => {
  assert.equal(nextLocalPort([{ localPort: 30001 }, { localPort: 30002 }]), 30003);
});

test("an older rule response cannot overwrite the list after a save", async () => {
  const pending = [];
  let rules;
  const refresh = createRulesRefresher(() => new Promise((resolve) => pending.push(resolve)), (items) => { rules = items; }, assert.fail);
  const older = refresh();
  const newer = refresh();
  pending[1](["saved-rule"]);
  await newer;
  pending[0]([]);
  await older;
  assert.deepEqual(rules, ["saved-rule"]);
});

test("an old failed rule request cannot hide a newer success", async () => {
  const pending = [];
  const refresh = createRulesRefresher(() => new Promise((resolve, reject) => pending.push({ resolve, reject })), () => {}, assert.fail);
  const older = refresh();
  const newer = refresh();
  pending[1].resolve([]);
  await newer;
  pending[0].reject(new Error("old network error"));
  await older;
});
