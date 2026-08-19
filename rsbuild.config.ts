import { defineConfig } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginSvgr } from '@rsbuild/plugin-svgr'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'

export default defineConfig({
  html: { title: 'Windows Service Manager', favicon: './ui/assets/icon.svg' },
  source: { entry: { index: './ui/main.tsx' } },
  output: { cleanDistPath: process.env.NODE_ENV === 'production' },
  server: { host: '127.0.0.1', proxy: { '/api': { target: 'http://127.0.0.1:3483', ws: true } } },
  plugins: [pluginReact({ reactCompiler: true }), pluginTailwindcss(), pluginSvgr()],
})
