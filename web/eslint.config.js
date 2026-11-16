import js from "@eslint/js";
import comments from "@eslint-community/eslint-plugin-eslint-comments";
import reactHooks from "eslint-plugin-react-hooks";
import testingLibrary from "eslint-plugin-testing-library";
import globals from "globals";
import tseslint from "typescript-eslint";

// The no-restricted-syntax selectors for test files. Flat config does not
// merge one rule's options across config objects: a later object that
// matches the same files replaces these whole. So any later block that sets
// no-restricted-syntax for test files must spread this list into its own,
// or the sleep ban stops applying to the files it matches.
export const testRestrictedSyntax = [
  {
    selector:
      "NewExpression[callee.name='Promise'] CallExpression[callee.name='setTimeout']",
    message:
      "A test never sleeps: wait for what it raced instead. If the pause is deliberate, say why with `// eslint-disable-next-line no-restricted-syntax -- <reason>`.",
  },
];

export default tseslint.config(
  {
    ignores: [
      "src/api/wire.gen.ts",
      "src/api/core/wasm_exec.js",
      "dist/",
      "storybook-static/",
    ],
  },
  {
    files: ["**/*.ts", "**/*.tsx"],
    extends: [js.configs.recommended, tseslint.configs.recommended],
    plugins: {
      "react-hooks": reactHooks,
      "@eslint-community/eslint-comments": comments,
    },
    linterOptions: { reportUnusedDisableDirectives: "error" },
    languageOptions: { globals: { ...globals.browser } },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
      "@eslint-community/eslint-comments/require-description": "error",
    },
  },
  {
    files: [".storybook/**", "vite.config.ts", "vitest.e2e.config.ts", "src/e2e/**"],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    files: ["**/*.test.{ts,tsx}"],
    plugins: { "testing-library": testingLibrary },
    rules: {
      "testing-library/no-wait-for-multiple-assertions": "error",
      "testing-library/no-wait-for-side-effects": "error",
      "no-restricted-syntax": ["error", ...testRestrictedSyntax],
    },
  },
);
