import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const style = (name: string) => readFileSync(resolve(process.cwd(), "src/styles", name), "utf8");
const conversation = style("conversation.css");
const shell = style("shell.css");
const base = style("base.css");

describe("scroll-contained app shell", () => {
  it("locks the viewport and gives the transcript the vertical scroll boundary", () => {
    expect(base).toMatch(/body\s*\{[^}]*overflow:\s*hidden/s);
    expect(shell).toMatch(/\.app-shell\s*\{[^}]*height:\s*100dvh[^}]*overflow:\s*hidden/s);
    expect(shell).toMatch(/\.app-main\s*\{[^}]*min-height:\s*0[^}]*overflow:\s*hidden/s);
    expect(conversation).toMatch(/\.conversation-panel\s*\{[^}]*min-height:\s*0[^}]*overflow:\s*hidden/s);
    expect(conversation).toMatch(/\.message-scroll\s*\{[^}]*height:\s*0[^}]*overflow-y:\s*auto/s);
  });
});
