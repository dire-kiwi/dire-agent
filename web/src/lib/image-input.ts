import type { ImageAttachment } from "./protocol";

export interface ComposerImage extends ImageAttachment {
  id: string;
  previewURL: string;
  name: string;
  data: string;
}

export const maxComposerImages = 4;
export const maxComposerImageBytes = 5 * 1024 * 1024;
export const maxComposerImageTotalBytes = 10 * 1024 * 1024;
export const supportedImageTypes = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);

export async function prepareComposerImage(file: File): Promise<ComposerImage> {
  if (!supportedImageTypes.has(file.type)) throw new Error(`Unsupported image type: ${file.type || "unknown"}`);
  if (!file.size) throw new Error("The pasted image is empty.");
  if (file.size > maxComposerImageBytes) throw new Error("Each pasted image must be 5 MiB or smaller.");
  const bytes = new Uint8Array(await readFileBuffer(file));
  let binary = "";
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  const data = btoa(binary);
  const name = file.name || `pasted-image.${extensionForMime(file.type)}`;
  return {
    id: globalThis.crypto?.randomUUID?.() || `image-${Date.now()}-${Math.random()}`,
    name,
    mime_type: file.type,
    data,
    size: file.size,
    previewURL: `data:${file.type};base64,${data}`,
  };
}

function readFileBuffer(file: File): Promise<ArrayBuffer> {
  if (typeof file.arrayBuffer === "function") return file.arrayBuffer();
  return new Promise<ArrayBuffer>((resolve, reject) => {
    const reader = new FileReader();
    reader.addEventListener("load", () => {
      if (reader.result instanceof ArrayBuffer) resolve(reader.result);
      else reject(new Error("Could not read the pasted image."));
    }, { once: true });
    reader.addEventListener("error", () => reject(reader.error || new Error("Could not read the pasted image.")), { once: true });
    reader.readAsArrayBuffer(file);
  });
}

function extensionForMime(mime: string): string {
  if (mime === "image/jpeg") return "jpg";
  return mime.split("/")[1] || "png";
}
