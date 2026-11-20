import { themeLabels, useTheme, type ThemeChoice } from "../theme";

/** Three buttons: follow the system, light or dark. */
export function ThemePicker() {
  const [choice, setChoice] = useTheme();
  return (
    <div className="theme-picker" role="group" aria-label="Thème">
      {(Object.keys(themeLabels) as ThemeChoice[]).map((c) => (
        <button
          key={c}
          type="button"
          className={c === choice ? "active" : undefined}
          aria-pressed={c === choice}
          title={c === "auto" ? "Suivre le réglage du système" : undefined}
          onClick={() => setChoice(c)}
        >
          {themeLabels[c]}
        </button>
      ))}
    </div>
  );
}
