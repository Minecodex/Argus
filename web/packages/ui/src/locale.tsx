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
  let stored: string | null = null;
  try { stored = window.localStorage.getItem(LOCALE_STORAGE_KEY); } catch { /* Opaque template frames have no storage. */ }
  if (stored === "zh-CN" || stored === "en-US") return stored;
  return "zh-CN";
}

export function LocaleProvider({
  children,
  onLocaleChange,
  locale: controlledLocale,
  persist = true,
}: {
  children: ReactNode;
  onLocaleChange?: (locale: SupportedLocale) => void;
  locale?: SupportedLocale;
  persist?: boolean;
}) {
  const [preferredLocale, setLocaleState] = useState<SupportedLocale>(initialLocale);
  const locale = controlledLocale ?? preferredLocale;

  useEffect(() => {
    document.documentElement.lang = locale;
    onLocaleChange?.(locale);
  }, [locale, onLocaleChange]);

  const value = useMemo(
    () => ({
      locale,
      setLocale: (next: SupportedLocale) => {
        if (persist) { try { window.localStorage.setItem(LOCALE_STORAGE_KEY, next); } catch { /* Template locale is supplied by the host. */ } }
        setLocaleState(next);
      },
    }),
    [locale, persist],
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
