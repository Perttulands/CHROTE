/**
 * Dev mode can only name a component because the build was told to keep the
 * name, and nothing else in the suite can notice if that instruction is lost:
 * every other test runs unminified source, where names survive by default. So
 * this one runs the real minifier over a trivial module twice — once with the
 * output options the dashboard build ships, once with none — and shows the
 * difference. Delete the option from `vite.config.ts` and this fails; that is
 * the whole point of it.
 */

import { describe, expect, it } from 'vitest'
import { rolldown, type OutputOptions } from 'rolldown'
import { resolveConfig } from 'vite'

const SOURCE = 'function TileHeader(props) { return props }\nglobalThis.chrote = TileHeader\n'

async function minified(output: OutputOptions): Promise<string> {
  const bundle = await rolldown({
    input: 'entry',
    plugins: [{
      name: 'inline',
      resolveId: (id: string) => (id === 'entry' ? id : null),
      load: (id: string) => (id === 'entry' ? SOURCE : null),
    }],
  })
  try {
    const { output: chunks } = await bundle.generate({ format: 'es', minify: true, ...output })
    return chunks[0].code
  } finally {
    await bundle.close()
  }
}

describe('the production build', () => {
  it('keeps component names through minification', async () => {
    // Resolved from the project's own config file, so this reads what Vite
    // will actually hand the bundler rather than what a literal here claims.
    const config = await resolveConfig({ root: process.cwd() }, 'build', 'production', 'production')
    const output = config.build.rollupOptions?.output as OutputOptions

    expect(await minified(output)).toContain('function TileHeader')
    // The same module without those options: the name dev mode reads is gone.
    expect(await minified({})).not.toContain('TileHeader')
  })
})
