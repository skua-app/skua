import js from '@eslint/js'
import ts from 'typescript-eslint'
import svelte from 'eslint-plugin-svelte'
import globals from 'globals'

export default [
  js.configs.recommended,
  ...ts.configs.recommended,
  ...svelte.configs['flat/recommended'],
  {
    files: ['**/*.svelte'],
    languageOptions: {
      parserOptions: {
        parser: ts.parser
      }
    }
  },
  {
    // svelte-eslint-parser surfaces the generic type parameter in
    // `<script lang="ts" generics="T extends string">` as a plain identifier,
    // which trips no-undef. TypeScript via svelte-check still enforces undef
    // checks here. Scoped to this one file so the rule stays on everywhere else.
    files: ['src/lib/components/Segmented.svelte'],
    rules: {
      'no-undef': 'off'
    }
  },
  {
    languageOptions: {
      globals: { ...globals.browser, ...globals.node }
    }
  },
  {
    rules: {
      '@typescript-eslint/no-unused-vars': [
        'error',
        { varsIgnorePattern: '^_', argsIgnorePattern: '^_' }
      ]
    }
  },
  {
    ignores: ['build/', '.svelte-kit/', 'node_modules/']
  }
]
