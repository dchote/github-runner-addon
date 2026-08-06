import pluginVue from 'eslint-plugin-vue'

export default [
  ...pluginVue.configs['flat/essential'],
  {
    files: ['src/**/*.{js,vue}'],
    rules: {
      'vue/multi-word-component-names': 'off',
      'vue/valid-v-slot': ['error', { allowModifiers: true }],
      'no-unused-vars': ['warn', { argsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_' }],
    },
  },
  {
    ignores: ['dist/**', 'node_modules/**'],
  },
]
