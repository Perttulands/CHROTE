import { lazy, Suspense } from 'react'

const ModelViewer = lazy(() => import('./ModelViewer'))

export default function ModelPreview({ path, name }: { path: string; name: string }) {
  return <Suspense fallback={<p>Loading 3D viewer…</p>}><ModelViewer key={path} path={path} name={name} /></Suspense>
}
