"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const { request, retryDelay } = require("../server/cmd/ukinfo-server/web/http-client.js");

test("proxy HTML and malformed JSON produce readable errors without reflecting their body", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response("<html>private proxy detail</html>", { status: 502 }));
  await assert.rejects(request("/api/search"), (e) => e.status === 502 && e.retryable && !e.message.includes("private"));
  globalThis.fetch.mock.mockImplementation(async () => new Response("not JSON", { headers: { "content-type": "application/json" } }));
  await assert.rejects(request("/api/search"), (e) => e.code === "invalid_response");
});
test("structured fields, request reference and bounded cooldown survive errors", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response(JSON.stringify({ error: "Slow down", code: "rate_limited" }), {
    status: 429, headers: { "content-type": "application/json", "retry-after": "10", "x-request-id": "safe-reference" },
  }));
  await assert.rejects(request("/api/search"), (e) => e.retryAfter === 10 && e.requestId === "safe-reference" && e.code === "rate_limited");
  assert.equal(retryDelay("999999"), 3600);
  assert.equal(retryDelay("nonsense"), 0);
  assert.equal(retryDelay(new Date(12000).toUTCString(), 2000), 10);
});
test("cancellation remains cancellation and raw exports preserve numeric bytes", async (t) => {
  const controller = new AbortController();
  t.mock.method(globalThis, "fetch", async (_, options) => {
    assert.equal(options.signal, controller.signal);
    throw new DOMException("Cancelled", "AbortError");
  });
  await assert.rejects(request("/api/search", { signal: controller.signal }), { name: "AbortError" });
  const raw = '{"large":9007199254740993}\n';
  globalThis.fetch.mock.mockImplementation(async () => new Response(raw));
  assert.equal(await request("/api/export", { format: "text" }), raw);
});
test("network errors and integrity errors have distinct retry behaviour", async (t) => {
  t.mock.method(globalThis, "fetch", async () => { throw new TypeError("Failed to fetch"); });
  await assert.rejects(request("/api/search"), (e) => e.code === "network_error" && e.retryable);
  globalThis.fetch.mock.mockImplementation(async () => new Response(JSON.stringify({ error: "Integrity failure", code: "catalogue_integrity" }), { status: 503, headers: { "content-type": "application/json" } }));
  await assert.rejects(request("/api/search"), (e) => !e.retryable);
});
