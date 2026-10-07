import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { I18nProvider } from "react-aria-components";

export type SupportedLocale = "zh-CN" | "en-US";

const LOCALE_STORAGE_KEY = "argus.locale";
const LocaleContext = createContext<{
  locale: SupportedLocale;
  setLocale: (locale: SupportedLocale) => void;
} | null>(null);

function initialLocale(): SupportedLocale {
  if (typeof window === "undefined") return "zh-CN";
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored === "zh-CN" || stored === "en-US") return stored;
  return window.navigator.language.toLowerCase().startsWith("en")
    ? "en-US"
    : "zh-CN";
}

export function LocaleProvider({
  children,
  onLocaleChange,
}: {
  children: ReactNode;
  onLocaleChange?: (locale: SupportedLocale) => void;
}) {
  const [locale, setLocaleState] = useState<SupportedLocale>(initialLocale);

  useEffect(() => {
    document.documentElement.lang = locale;
    onLocaleChange?.(locale);
  }, [locale, onLocaleChange]);

  const value = useMemo(
    () => ({
      locale,
      setLocale: (next: SupportedLocale) => {
        window.localStorage.setItem(LOCALE_STORAGE_KEY, next);
        setLocaleState(next);
      },
    }),
    [locale],
  );

  return (
    <LocaleContext.Provider value={value}>
      <I18nProvider locale={locale}>{children}</I18nProvider>
    </LocaleContext.Provider>
  );
}

export function useLocale() {
  const value = useContext(LocaleContext);
  if (!value) throw new Error("useLocale must be used inside LocaleProvider");
  return value;
}

export function useUiText() {
  const context = useContext(LocaleContext);
  const locale =
    context?.locale ??
    (typeof document !== "undefined" &&
    document.documentElement.lang === "en-US"
      ? "en-US"
      : "zh-CN");
  return (zhCN: string, enUS: string) => (locale === "zh-CN" ? zhCN : enUS);
}
