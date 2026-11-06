import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import jsdoc from "eslint-plugin-jsdoc";
import globals from "globals";

// Doc-comment coverage is a build gate (see dev/dood/Dockerfile), not just a
// style nit, so `require-jsdoc`'s contexts are picked deliberately: exported
// surface area must be documented, nested/internal helpers need not be.
const exportedDeclarationContexts = [
  "ExportNamedDeclaration > FunctionDeclaration",
  "ExportDefaultDeclaration > FunctionDeclaration",
  "ExportNamedDeclaration > TSInterfaceDeclaration",
  "ExportNamedDeclaration > TSTypeAliasDeclaration",
  "ExportNamedDeclaration > VariableDeclaration > VariableDeclarator > ArrowFunctionExpression",
];

export default tseslint.config(
  { ignores: ["dist/**"] },
  js.configs.recommended,
  {
    files: ["src/**/*.{ts,tsx}"],
    extends: [...tseslint.configs.recommendedTypeChecked],
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
      globals: globals.browser,
    },
    plugins: { "react-hooks": reactHooks, jsdoc },
    rules: {
      ...reactHooks.configs["recommended-latest"].rules,
      // This codebase's fetch-on-mount-and-poll effects (and the iframe
      // visited/pruning updaters in App.tsx) synchronize React state with
      // the backend or with props - exactly the use this rule's own message
      // endorses - but its heuristic flags the setState call itself rather
      // than recognizing the pattern. Reworking every poll effect to dodge
      // a brand-new (v7) advisory rule is out of scope for this migration.
      "react-hooks/set-state-in-effect": "off",
      "@typescript-eslint/no-misused-promises": [
        "error",
        // React event handlers routinely are async; every one of them
        // already catches its own errors internally (try/catch + error
        // state), so there's no unhandled-rejection risk here to flag.
        { checksVoidReturn: { attributes: false } },
      ],
      "jsdoc/require-jsdoc": [
        "error",
        {
          // The plugin's own defaults require JSDoc on every
          // FunctionDeclaration regardless of export; turn those off and
          // rely solely on `contexts` so only exported surface area gates.
          require: {
            FunctionDeclaration: false,
            MethodDefinition: false,
            ClassDeclaration: false,
            ArrowFunctionExpression: false,
            FunctionExpression: false,
          },
          contexts: exportedDeclarationContexts,
          checkConstructors: false,
        },
      ],
    },
  },
  {
    // api.ts's methods are object-literal properties, not top-level function
    // declarations, so the generic contexts above don't reach them - without
    // this override the rule would silently skip the file most in need of docs.
    files: ["src/api.ts"],
    rules: {
      "jsdoc/require-jsdoc": [
        "error",
        {
          require: {
            FunctionDeclaration: false,
            MethodDefinition: false,
            ClassDeclaration: false,
            ArrowFunctionExpression: false,
            FunctionExpression: false,
          },
          contexts: [...exportedDeclarationContexts, "ExportNamedDeclaration > VariableDeclaration > VariableDeclarator > ObjectExpression > Property"],
        },
      ],
    },
  },
  {
    files: ["vite.config.ts"],
    extends: [...tseslint.configs.recommendedTypeChecked],
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
      globals: globals.node,
    },
  },
);
