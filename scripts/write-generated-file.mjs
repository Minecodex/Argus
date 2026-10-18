import { writeFile } from "node:fs/promises";
import { setTimeout } from "node:timers/promises";

// Windows scanners can briefly hold a generator's freshly closed output file.
// Retry only transient sharing errors, and preserve the original failure.
export async function writeGeneratedFile(
  path,
  data,
  encoding = "utf8",
  io = { writeFile, wait: setTimeout },
  platform = process.platform,
) {
  for (let attempt = 0; ; attempt++) {
    try {
      return await io.writeFile(path, data, encoding);
    } catch (error) {
      if (
        platform !== "win32" ||
        attempt >= 5 ||
        !["EBUSY", "EPERM", "UNKNOWN"].includes(error.code)
      )
        throw error;
      await io.wait(100 * 2 ** attempt);
    }
  }
}
