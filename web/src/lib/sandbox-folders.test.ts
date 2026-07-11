import { describe, expect, it } from "vitest";
import { formatAdditionalFolders, parseAdditionalFolders, sameAdditionalFolders } from "./sandbox-folders";

describe("sandbox folder helpers", () => {
  it("parses one canonical candidate per line and removes exact duplicates", () => {
    expect(parseAdditionalFolders(" /workspace/shared \n\n/workspace/docs\n/workspace/shared\n"))
      .toEqual(["/workspace/shared", "/workspace/docs"]);
  });

  it("formats and compares folder sets without making one the main folder", () => {
    expect(formatAdditionalFolders(["/workspace/shared", "/workspace/docs"]))
      .toBe("/workspace/shared\n/workspace/docs");
    expect(sameAdditionalFolders(["/workspace/docs", "/workspace/shared"], ["/workspace/shared", "/workspace/docs"]))
      .toBe(true);
  });
});
