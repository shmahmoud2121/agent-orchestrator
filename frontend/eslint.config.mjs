import tsParser from "@typescript-eslint/parser";

// TypeScript owns type, unused-symbol, return, and switch-flow checks in
// `npm run typecheck`. ESLint adds syntax-level correctness checks throughout
// the desktop source without imposing a new formatting convention.
export default [
	{ ignores: ["node_modules/**", ".vite/**", "out/**", "dist/**", "release/**", "src/docs/**"] },
	{
		files: ["src/**/*.{ts,tsx}", "*.{ts,mjs}", "makers/**/*.ts", "scripts/**/*.mjs"],
		languageOptions: { parser: tsParser, ecmaVersion: "latest", sourceType: "module" },
		linterOptions: { noInlineConfig: true, reportUnusedDisableDirectives: "off" },
		rules: {
			"no-debugger": "error",
			"no-duplicate-case": "error",
			"no-dupe-keys": "error",
			"no-sparse-arrays": "error",
		},
	},
];
