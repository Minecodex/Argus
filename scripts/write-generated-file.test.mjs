import assert from "node:assert/strict";
import test from "node:test";
import { writeGeneratedFile } from "./write-generated-file.mjs";

test("retries Windows sharing errors without changing generated content", async () => {
  const calls = [];
  const io = {
    writeFile: async (...args) => {
      calls.push(args);
      if (calls.length < 3)
        throw Object.assign(new Error("locked"), { code: "UNKNOWN" });
    },
    wait: async () => {},
  };
  await writeGeneratedFile("types.gen.go", "source", "utf8", io, "win32");
  assert.equal(calls.length, 3);
  for (const call of calls)
    assert.deepEqual(call, ["types.gen.go", "source", "utf8"]);
});

test("bounds retries and does not hide permanent or non-Windows failures", async () => {
  for (const [platform, code, expected] of [
    ["win32", "EBUSY", 6],
    ["win32", "ENOSPC", 1],
    ["linux", "UNKNOWN", 1],
  ]) {
    let calls = 0;
    const failure = Object.assign(new Error(code), { code });
    await assert.rejects(
      writeGeneratedFile(
        "file",
        "content",
        "utf8",
        {
          writeFile: async () => {
            calls++;
            throw failure;
          },
          wait: async () => {},
        },
        platform,
      ),
      (error) => error === failure,
    );
    assert.equal(calls, expected);
  }
});
