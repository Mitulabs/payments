import { defineConfig } from 'tsup'

export default defineConfig({
  entry: {
    index: 'src/index.ts',
    express: 'src/express.ts',
    fetch: 'src/fetch.ts',
    hono: 'src/hono.ts',
    next: 'src/next.ts',
    elysia: 'src/elysia.ts',
  },
  format: ['esm', 'cjs'],
  dts: true,
  sourcemap: true,
  clean: true,
  treeshake: true,
  external: ['express', 'hono', /^next\//, 'elysia'],
})
