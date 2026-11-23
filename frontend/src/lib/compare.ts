export interface CompareSide {
  cluster: string;
  topic: string;
  key: string;
}

export function compareUrl(left: CompareSide, right: CompareSide = { ...left, key: "" }): string {
  const params = new URLSearchParams();
  for (const [p, s] of [["l", left], ["r", right]] as const) {
    params.set(`${p}c`, s.cluster);
    params.set(`${p}t`, s.topic);
    params.set(`${p}k`, s.key);
  }
  return `/compare?${params}`;
}
