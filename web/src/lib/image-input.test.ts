import { describe, expect, it } from "vitest";
import { prepareComposerImage } from "./image-input";

describe("prepareComposerImage", () => {
  it("encodes a supported pasted image for the daemon", async () => {
    const file = new File([new Uint8Array([1, 2, 3, 4])], "paste.png", { type: "image/png" });
    const prepared = await prepareComposerImage(file);
    expect(prepared).toMatchObject({ name: "paste.png", mime_type: "image/png", data: "AQIDBA==", size: 4 });
    expect(prepared.previewURL).toBe("data:image/png;base64,AQIDBA==");
  });

  it("rejects non-image clipboard files", async () => {
    const file = new File(["text"], "notes.txt", { type: "text/plain" });
    await expect(prepareComposerImage(file)).rejects.toThrow("Unsupported image type");
  });
});
