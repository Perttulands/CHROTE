import { expect, test } from './fixtures'
import { mockApiRoutes } from './mock-api'

// Headless Chromium uses software WebGL for this real rendering journey.
test.use({ launchOptions: { args: ['--enable-unsafe-swiftshader'] }, allowedConsoleMessages: [/GL Driver Message .*GPU stall due to ReadPixels/] })

// A tetrahedron with an embedded position buffer: no network asset dependency.
function modelBytes() {
  const vertices = new Float32Array([0,1,0, -1,-1,1, 1,-1,1, 0,1,0, 1,-1,1, 0,-1,-1, 0,1,0, 0,-1,-1, -1,-1,1])
  const binary = Buffer.from(vertices.buffer)
  const json = Buffer.from(JSON.stringify({
    asset: { version: '2.0' }, scene: 0, scenes: [{ nodes: [0] }], nodes: [{ mesh: 0 }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0 }, material: 0 }] }],
    materials: [{ doubleSided: true, pbrMetallicRoughness: { baseColorFactor: [0.2,0.6,0.9,1], metallicFactor: 0 } }],
    buffers: [{ byteLength: binary.length }], bufferViews: [{ buffer: 0, byteLength: binary.length }],
    accessors: [{ bufferView: 0, componentType: 5126, count: 9, type: 'VEC3', min: [-1,-1,-1], max: [1,1,1] }],
  }))
  const padded = Buffer.concat([json, Buffer.alloc((4 - json.length % 4) % 4, 32)])
  const header = Buffer.alloc(20)
  header.writeUInt32LE(0x46546c67, 0); header.writeUInt32LE(2, 4)
  header.writeUInt32LE(28 + padded.length + binary.length, 8)
  header.writeUInt32LE(padded.length, 12); header.writeUInt32LE(0x4e4f534a, 16)
  const chunk = Buffer.alloc(8)
  chunk.writeUInt32LE(binary.length); chunk.writeUInt32LE(0x004e4942, 4)
  return Buffer.concat([header, padded, chunk, binary])
}

test('arrow through files, orbit and zoom a GLB, and report a broken model', async ({ page }) => {
  await mockApiRoutes(page)
  await page.route(/\/api\/files\/resources\/?$/, route => route.fulfill({ json: {
    isDir: true, items: ['a.glb', 'b.glb'].map(name => ({ name, size: 1000, isDir: false, modified: '', type: '' })),
  } }))
  await page.route('**/api/files/diff*', route => route.fulfill({ json: { repository: '', diff: '' } }))
  await page.route('**/api/files/raw/**', route => route.fulfill({ body: new URL(route.request().url()).pathname.endsWith('a.glb') ? modelBytes() : Buffer.from('broken') }))
  await page.goto('/')
  await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()
  const first = page.getByRole('treeitem', { name: 'File a.glb' })
  await first.click()
  const canvas = page.getByLabel('3D model a.glb')
  await expect(page.getByText('Drag to rotate', { exact: false })).toBeVisible()
  const original = await canvas.screenshot()
  const box = (await canvas.boundingBox())!
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + 100, box.y + box.height / 2 + 30, { steps: 8 })
  await page.mouse.up()
  await expect.poll(async () => (await canvas.screenshot()).equals(original)).toBe(false)
  const rotated = await canvas.screenshot()
  await page.mouse.wheel(0, -300)
  await expect.poll(async () => (await canvas.screenshot()).equals(rotated)).toBe(false)
  await first.focus()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('treeitem', { name: 'File b.glb' })).toBeFocused()
  await expect(page.getByRole('alert')).toContainText('Could not open 3D model')
  await page.keyboard.press('ArrowUp')
  await expect(first).toBeFocused()
  await expect(page.getByText('Drag to rotate', { exact: false })).toBeVisible()
})
