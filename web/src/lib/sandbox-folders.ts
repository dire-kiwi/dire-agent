export function parseAdditionalFolders(value: string): string[] {
  const seen = new Set<string>();
  const folders: string[] = [];
  for (const line of value.split(/\r?\n/)) {
    const folder = line.trim();
    if (!folder || seen.has(folder)) continue;
    seen.add(folder);
    folders.push(folder);
  }
  return folders;
}

export function formatAdditionalFolders(folders: readonly string[] | undefined): string {
  return (folders ?? []).join("\n");
}

export function sameAdditionalFolders(left: readonly string[], right: readonly string[]): boolean {
  if (left.length !== right.length) return false;
  const sortedLeft = [...left].sort();
  const sortedRight = [...right].sort();
  return sortedLeft.every((folder, index) => folder === sortedRight[index]);
}
