import { Moon, Sun } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useLayoutEffect,
  useState,
} from "react";
import { IconButton } from "./IconButton";

type Theme = "light" | "dark";
const ThemeContext = createContext<
  { theme: Theme; toggle: () => void } | undefined
>(undefined);

/** 画面全体の配色を所有し、手動で選ぶまでは OS の配色に追従する。 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [systemTheme, setSystemTheme] = useState<Theme>(() =>
    window.matchMedia?.("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light",
  );
  const [selectedTheme, setSelectedTheme] = useState<Theme>();
  const theme = selectedTheme ?? systemTheme;

  useEffect(() => {
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (media === undefined) return;
    const update = () => setSystemTheme(media.matches ? "dark" : "light");
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useLayoutEffect(() => {
    const root = document.documentElement;
    // 配色の変更を描画する間は全要素の色を即時更新し、通常の操作ではアニメーションを戻す。
    root.dataset.themeChanging = "true";
    root.dataset.theme = theme;
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => {
        delete root.dataset.themeChanging;
      });
    });
    return () => {
      cancelAnimationFrame(frame);
      delete root.dataset.themeChanging;
      delete root.dataset.theme;
    };
  }, [theme]);

  return (
    <ThemeContext
      value={{
        theme,
        toggle: () => setSelectedTheme(theme === "dark" ? "light" : "dark"),
      }}
    >
      {children}
    </ThemeContext>
  );
}

/** ヘッダーから太陽・三日月のアイコンで配色を切り替える。 */
export function ThemeToggle() {
  const context = useContext(ThemeContext);
  if (context === undefined) return null;
  const isDark = context.theme === "dark";
  return (
    <IconButton
      label={isDark ? "ライトテーマに切り替え" : "ダークテーマに切り替え"}
      onPress={context.toggle}
      className="ml-auto shrink-0"
    >
      {isDark ? (
        <Moon size={16} aria-hidden="true" />
      ) : (
        <Sun size={16} aria-hidden="true" />
      )}
    </IconButton>
  );
}
