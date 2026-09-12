import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";

const apps = ["enterprise", "platform", "card-runtime"];
const forbidden = [
  "argus-mock:",
  "createMockApiClient",
  "host-cache-bj-01",
  "payment-worker",
  "sre-schedule",
  ".argus.invalid",
];
const failures = [];

for (const app of apps) {
  const files = readdirSync(`web/apps/${app}/dist`, { recursive: true, withFileTypes: true });
  for (const entry of files) {
    if (!entry.isFile() || !entry.name.endsWith(".js")) continue;
    const file = path.join(entry.parentPath, entry.name);
    const source = readFileSync(file, "utf8");
    for (const marker of forbidden) {
      if (source.includes(marker)) {
        failures.push(`${app}: ${marker} found in ${file}`);
      }
    }
  }
}

if (failures.length > 0) {
  throw new Error(`Real frontend build contains mock data or placeholder endpoints:\n${failures.join("\n")}`);
}

console.log("Real frontend bundles contain no mock seed markers or placeholder endpoints");
