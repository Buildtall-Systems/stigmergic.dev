export function fileURLToPath(url: string): string {
  return url.replace("file://", "");
}
