import { expect, it } from "vitest";
import { alertMeasurement } from "./measurement";

it("formats floating point jitter without losing tiny values or threshold differences", () => {
  expect(alertMeasurement(0.4999750012499375, "req/s", "en-US", 0.1)).toBe(
    "0.5 req/s",
  );
  expect(alertMeasurement(0.00000001, "", "zh-CN")).toBe("0.00000001");
  expect(alertMeasurement(0.10000001, "", "en-US", 0.1)).not.toBe(
    alertMeasurement(0.1, "", "en-US", 0.10000001),
  );
  expect(alertMeasurement(null, "req/s", "en-US")).toBe("—");
});
