import js from '@eslint/js';
import globals from 'globals';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import reactHooks from 'eslint-plugin-react-hooks';
import tseslint from 'typescript-eslint';

export default [
  {
    ignores: [
      'coverage/**',
      'dist/**',
      'node_modules/**',
      'playwright-report/**',
      'test-results/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{js,jsx,ts,tsx}', 'e2e/**/*.{js,jsx,ts,tsx}'],
    languageOptions: {
      globals: {...globals.browser, ...globals.es2022},
      parserOptions: {ecmaFeatures: {jsx: true}},
    },
    plugins: {
      'jsx-a11y': jsxA11y,
      'react-hooks': reactHooks,
    },
    rules: {
      ...jsxA11y.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      // Initial data fetches intentionally drive loading state from an effect.
      'react-hooks/set-state-in-effect': 'off',
    },
  },
  {
    files: ['*.config.js', 'e2e/**/*.{js,ts}', 'scripts/**/*.mjs'],
    languageOptions: {globals: {...globals.node, ...globals.browser}},
  },
];
