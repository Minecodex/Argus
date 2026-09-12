import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (mode !== "mock" && mode !== "real") {
  console.error("usage: node scripts/build-web.mjs mock|real");
  process.exit(2);
}

const env = { ...process.env, VITE_API_MODE: mode };
if (mode === "real") {
  // Deployed portals proxy /api through their own origin. Card and Platform
  // origins are supplied by /argus-runtime.json, unless explicitly overridden.
  env.VITE_API_BASE_URL ??= "/";
}

const windows = process.platform === "win32";
const executable = windows ? (process.env.ComSpec ?? "cmd.exe") : "pnpm";
const args = windows
  ? ["/d", "/s", "/c", "pnpm.cmd -r --if-present build"]
  : ["-r", "--if-present", "build"];
const child = spawn(executable, args, {
  cwd: process.cwd(),
  env,
  stdio: "inherit",
  windowsHide: true,
});

child.on("error", (error) => {
  console.error(error.message);
  process.exit(1);
});
child.on("exit", (code, signal) => {
  if (signal) {
    console.error(`web build terminated by ${signal}`);
    process.exit(1);
  }
  if (code === 0 && mode === "real") {
    const check = spawnSync(process.execPath, [fileURLToPath(new URL("./check-real-build.mjs", import.meta.url))], {
      cwd: process.cwd(), env, stdio: "inherit", windowsHide: true,
    });
    if (check.error) console.error(check.error.message);
    process.exit(check.status ?? 1);
  }
  process.exit(code ?? 1);
});
