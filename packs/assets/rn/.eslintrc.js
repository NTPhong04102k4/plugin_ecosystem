// ESLint config — React Native + TypeScript. Extends the official @react-native
// preset (the RN ecosystem standard) with stricter overrides. Shipped by
// skillrunner's `rn` pack (`sr apply-base --stack rn`).
//
// Requires devDependencies: eslint  @react-native/eslint-config  prettier
module.exports = {
  root: true,
  extends: '@react-native',
  rules: {
    // Strict-but-pragmatic tightening on top of @react-native.
    'no-console': ['warn', { allow: ['warn', 'error'] }],
    'react-native/no-inline-styles': 'error',
    'react-native/no-unused-styles': 'error',
    '@typescript-eslint/no-unused-vars': [
      'error',
      { argsIgnorePattern: '^_', varsIgnorePattern: '^_', ignoreRestSiblings: true },
    ],
    'react-hooks/exhaustive-deps': 'error',
    eqeqeq: ['error', 'always', { null: 'ignore' }],
  },
}
