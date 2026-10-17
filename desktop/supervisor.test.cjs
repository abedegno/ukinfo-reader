"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const { startServer } = require("./supervisor.cjs");

test("server startup, health and stdin-owned shutdown", async () => {
  const script = `const http = require('node:http');
    const s = http.createServer((req, res) => {res.end('{}');}).listen(0, '127.0.0.1', () => console.log(JSON.stringify({url:'http://127.0.0.1:' + s.address().port})));
    process.stdin.resume(); process.stdin.on('end', () => s.close());`;
  const server = await startServer(process.execPath, ["-e", script]);
  assert.match(server.address, /^http:\/\/127\.0\.0\.1:\d+$/);
  await server.stop();
  await server.stop();
});
test("rejects startup failures and non-loopback addresses", async () => {
  await assert.rejects(
    startServer(process.execPath, ["-e", "process.exit(7)"]),
    /exited during startup/,
  );
  const script = `console.log(JSON.stringify({url:'http://example.com:1234'}));process.stdin.resume();process.stdin.on('end',()=>process.exit());`;
  await assert.rejects(
    startServer(process.execPath, ["-e", script]),
    /Invalid local server address/,
  );
});
test("missing executable fails promptly", async () => {
  await assert.rejects(
    startServer("/definitely-not-an-ukinfo-binary", []),
    /ENOENT/,
  );
});
