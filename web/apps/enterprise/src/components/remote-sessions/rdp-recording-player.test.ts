import { describe, expect, it } from "vitest";

import { guacamoleRecordingStream } from "./rdp-recording-player";

describe("guacamoleRecordingStream", () => {
  it("reconstructs only the ordered guacd output stream", () => {
    expect(
      guacamoleRecordingStream([
        { time: 0, type: "i", data: "3.key,1.1;" },
        { time: 0.1, type: "o", data: "4.size,3.800,3.600;" },
        { time: 0.2, type: "m", data: { state: "active" } },
        { time: 0.3, type: "o", data: "4.sync,2.10;" },
      ]),
    ).toBe("4.size,3.800,3.600;4.sync,2.10;");
  });
});
