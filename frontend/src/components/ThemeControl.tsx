import {useEffect, useState} from 'react';

export function ThemeControl() {
  const [theme, setTheme] = useState(() => {
    try {
      return localStorage.getItem('forgequeue-theme') === 'light' ? 'light' : 'dark';
    } catch {
      return 'dark';
    }
  });
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem('forgequeue-theme', theme);
    } catch {
      /* Theme still works without storage. */
    }
  }, [theme]);
  return (
    <button
      aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}
      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
    >
      {theme === 'dark' ? 'Light theme' : 'Dark theme'}
    </button>
  );
}
