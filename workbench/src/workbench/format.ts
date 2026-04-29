import type { Lang } from "./types";

const zhLocale = "zh-CN";
const enLocale = "en-US";

const localeFor = (lang: Lang) => (lang === "zh" ? zhLocale : enLocale);

export const toMultiline = (value: string[]) => value.join("\n");

export const initialLang = (): Lang => {
  const saved = localStorage.getItem("roodox.workbench.lang");
  return saved === "zh" || saved === "en" ? saved : "zh";
};

export const errorText = (error: unknown) => (error instanceof Error ? error.message : String(error));

export const formatTime = (value: number | undefined, lang: Lang, fallback: string) =>
  !value || value <= 0
    ? fallback
    : new Intl.DateTimeFormat(localeFor(lang), {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit"
      }).format(new Date(value * 1000));

export const formatNumber = (value: number | undefined, lang: Lang, fallback: string) =>
  value === undefined || value === null || Number.isNaN(value)
    ? fallback
    : new Intl.NumberFormat(localeFor(lang)).format(value);

export const formatBytes = (value: number | undefined, lang: Lang, fallback: string) => {
  if (value === undefined || value === null || Number.isNaN(value)) return fallback;
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = Math.abs(value);
  let idx = 0;
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024;
    idx += 1;
  }
  return `${new Intl.NumberFormat(localeFor(lang), { maximumFractionDigits: size >= 10 || idx === 0 ? 0 : 1 }).format(size)} ${units[idx]}`;
};

export const formatMillis = (value: number | undefined, lang: Lang, fallback: string) =>
  value === undefined || value === null || Number.isNaN(value)
    ? fallback
    : value >= 1000
      ? `${new Intl.NumberFormat(localeFor(lang), { maximumFractionDigits: value >= 10000 ? 0 : 1 }).format(value / 1000)} s`
      : `${formatNumber(value, lang, fallback)} ms`;

export const formatInterval = (value: number | undefined, lang: Lang, fallback: string) => {
  if (!value || value <= 0) return fallback;
  if (value % 3600 === 0) return `${formatNumber(value / 3600, lang, fallback)} h`;
  if (value % 60 === 0) return `${formatNumber(value / 60, lang, fallback)} min`;
  return `${formatNumber(value, lang, fallback)} s`;
};

export const isMountedState = (value: string) => ["mounted", "ready", "active"].includes(value.trim().toLowerCase());
