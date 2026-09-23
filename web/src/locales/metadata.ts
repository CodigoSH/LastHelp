/** A language this build ships, named the way its own speakers name it. */
export interface LocaleMetadata {
  code: string;
  /** The endonym. A picker that says "Bengali" is a picker a Bengali speaker
   *  has to read in English to find their own language. */
  name: string;
  flag: string;
}

/** The twenty, each with the flag Lastboard shows beside it in its language
 *  picker — the same flag for the same language in both products, English's
 *  included. This list and the server's have to agree, and a test compares
 *  them: two hand-written lists drift the first time one gains a language. */
export const availableLocales: LocaleMetadata[] = [
  { code: "en", name: "English", flag: "🇺🇸" },
  { code: "es", name: "Español", flag: "🇪🇸" },
  { code: "de", name: "Deutsch", flag: "🇩🇪" },
  { code: "fr", name: "Français", flag: "🇫🇷" },
  { code: "it", name: "Italiano", flag: "🇮🇹" },
  { code: "pt", name: "Português", flag: "🇵🇹" },
  { code: "nl", name: "Nederlands", flag: "🇳🇱" },
  { code: "pl", name: "Polski", flag: "🇵🇱" },
  { code: "tr", name: "Türkçe", flag: "🇹🇷" },
  { code: "el", name: "Ελληνικά", flag: "🇬🇷" },
  { code: "ru", name: "Русский", flag: "🇷🇺" },
  { code: "zh", name: "简体中文", flag: "🇨🇳" },
  { code: "ja", name: "日本語", flag: "🇯🇵" },
  { code: "ko", name: "한국어", flag: "🇰🇷" },
  { code: "hi", name: "हिन्दी", flag: "🇮🇳" },
  { code: "bn", name: "বাংলা", flag: "🇧🇩" },
  { code: "id", name: "Bahasa Indonesia", flag: "🇮🇩" },
  { code: "ar", name: "العربية", flag: "🇸🇦" },
  { code: "fa", name: "فارسی", flag: "🇮🇷" },
  { code: "ur", name: "اردو", flag: "🇵🇰" },
];

/** The three that read right to left. */
export const rtlLocales = ["ar", "fa", "ur"] as const;

export function directionOf(code: string): "ltr" | "rtl" {
  return (rtlLocales as readonly string[]).includes(code) ? "rtl" : "ltr";
}
