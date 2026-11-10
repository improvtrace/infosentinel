/** @type {import('tailwindcss').Config} */
export default {
  // 主题切换由 CSS 变量承担(@media prefers-color-scheme),不再依赖 dark: 前缀
  darkMode: 'media',
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // 语义令牌:亮/暗主题各自映射到同一套语义,页面代码不感知主题
        bg: 'rgb(var(--c-bg) / <alpha-value>)',
        surface: 'rgb(var(--c-surface) / <alpha-value>)',
        raised: 'rgb(var(--c-raised) / <alpha-value>)',
        ink: 'rgb(var(--c-ink) / <alpha-value>)',
        dim: 'rgb(var(--c-dim) / <alpha-value>)',
        ghost: 'rgb(var(--c-ghost) / <alpha-value>)',
        accent: 'rgb(var(--c-accent) / <alpha-value>)',
        strong: 'rgb(var(--c-strong) / <alpha-value>)',
        ok: 'rgb(var(--c-ok) / <alpha-value>)',
        warn: 'rgb(var(--c-warn) / <alpha-value>)',
        bad: 'rgb(var(--c-bad) / <alpha-value>)',
        // 预混合的结构色(细线 / 软底),不走 alpha
        line: 'var(--line)',
        'line-strong': 'var(--line-strong)',
        'code-bg': 'var(--code-bg)',
        'soft-accent': 'var(--soft-accent)',
        'soft-ok': 'var(--soft-ok)',
        'soft-warn': 'var(--soft-warn)',
        'soft-bad': 'var(--soft-bad)',
        hover: 'var(--hover)',
      },
      fontFamily: {
        sans: [
          'IBM Plex Sans',
          '-apple-system',
          'BlinkMacSystemFont',
          'PingFang SC',
          'HarmonyOS Sans SC',
          'MiSans',
          'Microsoft YaHei',
          'system-ui',
          'sans-serif',
        ],
        mono: ['IBM Plex Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      maxWidth: {
        content: '76rem',
      },
    },
  },
  plugins: [],
}
