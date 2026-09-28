import { describe, expect, it } from "vitest";
import { mentionQuery } from "./mention-query";

describe("structured resource mention editing", () => {
  it.each(["Argus production overview", "生产 环境总览"])(
    "selects a complete multiword name: %s",
    (name) => {
      const prefix = "请检查 ",
        suffix = "，然后继续";
      const value = `${prefix}@${name}${suffix}`;
      const selected = mentionQuery(value, prefix.length + name.length + 1)!;
      expect(selected.query).toBe(name);
      expect(value.slice(0, selected.start) + value.slice(selected.end)).toBe(
        prefix + suffix,
      );
    },
  );
  it("keeps mentions on one line and requires a word boundary", () => {
    expect(mentionQuery("name@example.com", 16)).toBeNull();
    expect(mentionQuery("@service\ncheck", 14)).toBeNull();
    expect(mentionQuery("@" + "a".repeat(257), 258)).toBeNull();
    const text = "first @old then @new view";
    expect(mentionQuery(text, text.length)?.query).toBe("new view");
  });
});
