export type OverlayPreset = "" | "direct" | "tailscale" | "easytier" | "custom";

type JsonObject = Record<string, unknown>;

export function normalizeOverlayProvider(value: string): string {
  return value.trim().toLowerCase();
}

export function detectOverlayPreset(value: string): OverlayPreset {
  const normalized = normalizeOverlayProvider(value);
  if (!normalized) return "";
  if (normalized === "direct" || normalized === "tailscale" || normalized === "easytier") return normalized;
  return "custom";
}

export function parseOverlayJoinConfig(input: string): { value: JsonObject; error: string | null } {
  const trimmed = input.trim();
  if (!trimmed) return { value: {}, error: null };
  try {
    const parsed = JSON.parse(trimmed);
    if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
      return { value: {}, error: "Overlay JSON must be a JSON object." };
    }
    return { value: parsed as JsonObject, error: null };
  } catch (error) {
    return {
      value: {},
      error: error instanceof Error ? error.message : "Overlay JSON is invalid."
    };
  }
}

function compactObject(value: JsonObject): JsonObject {
  return Object.fromEntries(
    Object.entries(value).filter(([, entry]) => {
      if (entry === null || entry === undefined) return false;
      if (typeof entry === "string") return entry.trim().length > 0;
      if (Array.isArray(entry)) return entry.length > 0;
      return true;
    })
  );
}

export function stringifyOverlayJoinConfig(value: JsonObject): string {
  const compacted = compactObject(value);
  if (Object.keys(compacted).length === 0) return "{}";
  return JSON.stringify(compacted, null, 2);
}

export function getOverlayStringField(input: string, key: string): string {
  const parsed = parseOverlayJoinConfig(input);
  const value = parsed.value[key];
  return typeof value === "string" ? value : "";
}

export function getOverlayStringListField(input: string, key: string): string[] {
  const parsed = parseOverlayJoinConfig(input);
  const value = parsed.value[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item.trim().length > 0) : [];
}

export function setOverlayStringField(input: string, key: string, value: string): string {
  const parsed = parseOverlayJoinConfig(input);
  const next = { ...parsed.value };
  const trimmed = value.trim();
  if (trimmed) next[key] = trimmed;
  else delete next[key];
  return stringifyOverlayJoinConfig(next);
}

export function setOverlayStringListField(input: string, key: string, value: string): string {
  const parsed = parseOverlayJoinConfig(input);
  const next = { ...parsed.value };
  const items = value
    .split(/\r?\n|,/)
    .map((entry) => entry.trim())
    .filter(Boolean);
  if (items.length > 0) next[key] = items;
  else delete next[key];
  return stringifyOverlayJoinConfig(next);
}
