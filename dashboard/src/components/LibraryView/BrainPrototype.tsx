// Throwaway: three ways to find understanding and follow evidence, on Library.
// ?prototype=brain&variant=A|B|C. All content and search come from the real brain.
import { memo, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import './BrainPrototype.css'

type Kind = 'topic' | 'research' | 'note' | 'source' | 'self'
type Summary = { slug: string; title: string; kind: Kind; summary: string; collection: string; facets: string[]; updated?: string }
type Page = Summary & { content: string; related: (Summary & { relation: 'links_to' | 'linked_from' })[]; source?: { origin_path?: string; url?: string; captured?: string; sha256?: string } }
type Catalog = { topics: Summary[]; facets: string[]; recent: Summary[]; counts: { knowledge: number; self: number } }
type Nav = { slug: string; q: string; collection: string; facet: string; variant: string }
type Source = { status: string; content?: string; next_offset?: number; truncated?: boolean; total_chars?: number; sha256?: string; reference?: string; reason?: string; message?: string }
type Answer = { question: string; answer: string; scope: string }
const variants = [{ key: 'A', name: 'Reading room' }, { key: 'B', name: 'Explorer' }, { key: 'C', name: 'Research desk' }]
const label = (s: string) => s.replace(/-/g, ' ').replace(/^./, c => c.toUpperCase())
const message = (e: unknown) => e instanceof Error ? e.message : String(e)
const readNav = (): Nav => {
  const p = new URLSearchParams(location.search)
  return { slug: p.get('page') || '', q: p.get('q') || '', collection: p.get('collection') === 'self' ? 'self' : 'knowledge', facet: p.get('facet') || '', variant: ['A', 'B', 'C'].includes(p.get('variant') || '') ? p.get('variant')! : 'A' }
}
async function api<T>(route: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/brain-prototype/${route}`, init)
  const body = await response.json()
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`)
  return body
}

function BrainPrototype({ active }: { active: boolean }) {
  const [nav, setNav] = useState(readNav)
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [catalogError, setCatalogError] = useState('')
  const [revision, setRevision] = useState(0)
  const [results, setResults] = useState<Summary[] | null>(null)
  const [searchError, setSearchError] = useState('')
  const [page, setPage] = useState<Page | null>(null)
  const [pageError, setPageError] = useState('')
  const [query, setQuery] = useState(nav.q)
  const [mode, setMode] = useState<'search' | 'ask'>('search')
  const [askOpen, setAskOpen] = useState(false)
  const [question, setQuestion] = useState('')
  const [askScope, setAskScope] = useState(false)
  const [asking, setAsking] = useState(false)
  const [answer, setAnswer] = useState<Answer | null>(null)
  const [askError, setAskError] = useState('')
  const [source, setSource] = useState<Source | null>(null)
  const [sourceBusy, setSourceBusy] = useState(false)
  const [sourceError, setSourceError] = useState('')
  const [notice, setNotice] = useState('')
  const pages = useRef(new Map<string, Page>())
  const scrolls = useRef(new Map<string, number>())
  const reader = useRef<HTMLDivElement>(null)
  const searchInput = useRef<HTMLInputElement>(null)
  const listing = useRef<HTMLDivElement>(null)
  const listKey = `list:${nav.variant}:${nav.q}:${nav.collection}:${nav.facet}`
  const sourceRequest = useRef(0)
  const sequence = useRef(0)

  function go(patch: Partial<Nav>, replace = false) {
    if (reader.current && nav.slug) scrolls.current.set(nav.slug, reader.current.scrollTop)
    const next = { ...nav, ...patch }
    const url = new URL(location.href)
    for (const [key, value] of Object.entries({ page: next.slug, q: next.q, collection: next.collection, facet: next.facet, variant: next.variant })) {
      if (value) url.searchParams.set(key, value)
      else url.searchParams.delete(key)
    }
    history[replace ? 'replaceState' : 'pushState']({ brainDepth: (history.state?.brainDepth || 0) + (replace ? 0 : 1) }, '', url)
    setNav(next)
  }
  const open = (slug: string) => go({ slug })

  async function loadCatalog(refresh = false) {
    setCatalogError('')
    try {
      setCatalog(await api<Catalog>(`catalog${refresh ? '?refresh=1' : ''}`))
      if (refresh) { pages.current.clear(); setRevision(value => value + 1) }
    }
    catch (e) { setCatalogError(message(e)) }
  }
  useEffect(() => { void loadCatalog() }, [])
  useEffect(() => {
    const pop = () => setNav(readNav())
    window.addEventListener('popstate', pop)
    return () => window.removeEventListener('popstate', pop)
  }, [])
  useEffect(() => { setQuery(nav.q) }, [nav.q])
  useEffect(() => {
    const abort = new AbortController()
    setSearchError('')
    setResults(null)
    if (!nav.q) return
    const params = new URLSearchParams({ q: nav.q, collection: nav.collection, facet: nav.facet })
    api<{ results: Summary[] }>(`search?${params}`, { signal: abort.signal }).then(data => setResults(data.results)).catch(e => { if (!abort.signal.aborted) setSearchError(message(e)) })
    return () => abort.abort()
  }, [nav.q, nav.collection, nav.facet, revision])
  useEffect(() => {
    const abort = new AbortController()
    sourceRequest.current++
    setSource(null); setSourceError(''); setSourceBusy(false); setPageError(''); setNotice('')
    if (!nav.slug) { setPage(null); return }
    const saved = pages.current.get(nav.slug)
    setPage(saved || null)
    if (saved) return
    api<Page>(`page?slug=${encodeURIComponent(nav.slug)}`, { signal: abort.signal }).then(data => {
      pages.current.set(nav.slug, data); setPage(data)
    }).catch(e => { if (!abort.signal.aborted) setPageError(message(e)) })
    return () => abort.abort()
  }, [nav.slug, revision])
  useLayoutEffect(() => {
    if (reader.current) reader.current.scrollTop = scrolls.current.get(nav.slug) || 0
    if (listing.current) listing.current.scrollTop = scrolls.current.get(listKey) || 0
  }, [page?.slug, nav.variant, nav.slug, listKey])
  useEffect(() => {
    if (!active) return
    const key = (e: KeyboardEvent) => {
      if (e.target instanceof Element && e.target.closest('input,textarea,select,[contenteditable="true"]')) return
      if (e.altKey || e.ctrlKey || e.metaKey) return
      if (e.key === '/') { e.preventDefault(); searchInput.current?.focus() }
      if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
        e.preventDefault()
        const i = variants.findIndex(v => v.key === nav.variant)
        go({ variant: variants[(i + (e.key === 'ArrowRight' ? 1 : 2)) % 3].key }, true)
      }
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  })

  async function ask(text = question || query) {
    if (!text.trim() || asking) return
    const request = ++sequence.current
    const scoped = askScope && page ? page : null
    setQuestion(text); setAskOpen(true); setAsking(true); setAskError('')
    try {
      const data = await api<{ answer: string }>('ask', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ question: text, ...(scoped ? { slug: scoped.slug } : {}) }) })
      if (request === sequence.current) setAnswer({ question: text, answer: data.answer, scope: scoped?.title || 'Across the brain' })
    } catch (e) { setAskError(message(e)) }
    finally { setAsking(false) }
  }
  async function readSource(more = false) {
    if (!page || sourceBusy) return
    const request = ++sourceRequest.current
    setSourceBusy(true); setSourceError('')
    try {
      const data = await api<Source>(`source?slug=${encodeURIComponent(page.slug)}&offset=${more ? source?.next_offset || 0 : 0}`)
      if (request === sourceRequest.current) setSource(previous => ({ ...data, content: more ? (previous?.content || '') + (data.content || '') : data.content }))
    } catch (e) { if (request === sourceRequest.current) setSourceError(message(e)) }
    finally { if (request === sourceRequest.current) setSourceBusy(false) }
  }
  function submit(e: FormEvent) {
    e.preventDefault()
    if (mode === 'ask') void ask(query)
    else go({ q: query.trim(), slug: '' })
  }
  function rich(text: string): ReactNode {
    const linked = text.replace(/\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|([^\]]+))?\]\]/g, (_all, slug: string, title?: string) => {
      const known = page?.related.find(p => p.slug === slug) || catalog?.topics.find(p => p.slug === slug)
      return `[${title || known?.title || label(slug)}](#brain/${encodeURIComponent(slug)})`
    })
    return <ReactMarkdown remarkPlugins={[remarkGfm]} components={{
      a: ({ href, children }) => <a href={href} onClick={e => {
        if (href?.startsWith('#brain/')) { e.preventDefault(); open(decodeURIComponent(href.slice(7))) }
        else if (href?.startsWith('/') || href?.startsWith('file:')) { e.preventDefault(); setNotice(`Local reference: ${href}`) }
      }} {...(href?.startsWith('http') ? { target: '_blank', rel: 'noreferrer' } : {})}>{children}</a>,
    }}>{linked}</ReactMarkdown>
  }
  const topics = (catalog?.topics || []).filter(p => p.collection === nav.collection && (!nav.facet || p.facets.includes(nav.facet)))
  const listed = nav.q ? results || [] : topics
  const current = variants.find(v => v.key === nav.variant)!
  const isReading = Boolean(nav.slug)

  function topicList(items: Summary[], compact = false) {
    return <div className={compact ? 'bp-list bp-list-compact' : 'bp-list'}>{items.map((item, i) => <button key={item.slug} className={`bp-entry ${nav.slug === item.slug ? 'selected' : ''}`} onClick={() => open(item.slug)}>
      {!compact && <span className="bp-number">{String(i + 1).padStart(2, '0')}</span>}
      <span className="bp-entry-copy"><span className="bp-kind">{item.kind}</span><strong>{item.title || label(item.slug)}</strong><span className="bp-summary">{item.summary}</span></span><span className="bp-enter" aria-hidden="true">↗</span>
    </button>)}</div>
  }
  const searchForm = <form className="bp-search" onSubmit={submit}>
    <span aria-hidden="true" className="bp-search-icon">⌕</span>
    <input ref={searchInput} aria-label="Search or ask the brain" value={query} onChange={e => setQuery(e.target.value)} placeholder={mode === 'ask' ? 'What would you like to understand?' : 'Find a subject, lesson, or source…'} />
    <select aria-label="Search mode" value={mode} onChange={e => setMode(e.target.value as 'search' | 'ask')}><option value="search">Search</option><option value="ask">Ask</option></select>
    <button className="bp-primary" disabled={!query.trim() || (mode === 'ask' && asking)} type="submit">{mode === 'ask' ? 'Ask →' : 'Find →'}</button>
  </form>
  const filters = <>
    <div className="bp-collections" aria-label="Collections">{['knowledge', 'self'].map(c => <button key={c} aria-pressed={nav.collection === c} onClick={() => go({ collection: c, facet: '', slug: '' })}>{label(c)} <span>{catalog?.counts[c as 'knowledge' | 'self']?.toLocaleString()}</span></button>)}</div>
    <label className="bp-facet">Applies to <select aria-label="Filter by field" value={nav.facet} onChange={e => go({ facet: e.target.value, slug: '' })}><option value="">All fields</option>{catalog?.facets.map(f => <option key={f} value={f}>{label(f)}</option>)}</select></label>
  </>
  const resultHeading = <div className="bp-section-heading"><span>{nav.q ? `Results for “${nav.q}”` : nav.collection === 'self' ? 'About you' : 'Explore our understanding'}</span><span>{!catalog ? 'Loading…' : nav.q && results === null ? 'Searching…' : `${listed.length} ${nav.q ? 'results' : nav.collection === 'self' ? 'pages' : 'topics'}`}</span></div>
  const empty = catalog && !listed.length && (results !== null || !nav.q) ? <div className="bp-empty"><h3>{nav.q ? 'Nothing found with those words.' : 'No topics in this selection.'}</h3><p>{nav.q ? 'Try a shorter phrase, another name, or a different field. Search matches the words in the brain.' : 'Search this collection to find its research, notes, and sources.'}</p>{nav.facet && <button onClick={() => go({ facet: '' })}>Clear field filter</button>}</div> : null
  const errors = <>{catalogError && <div className="bp-error" role="alert">{catalogError}<button onClick={() => void loadCatalog()}>Try again</button></div>}{searchError && <p className="bp-error" role="alert">{searchError}</p>}</>

  const askPanel = <section className="bp-ask-panel" aria-label="Ask the brain">
    <div className="bp-section-heading"><span>Ask the brain</span>{nav.variant !== 'C' && <button onClick={() => setAskOpen(false)}>Close</button>}</div>
    <p className="bp-muted">A sourced answer, with the pages behind it.</p>
    <form onSubmit={e => { e.preventDefault(); void ask() }}>
      <textarea aria-label="Your question" value={question} onChange={e => setQuestion(e.target.value)} placeholder="What have we learned about agent reviews?" rows={3} />
      <div className="bp-ask-actions"><label><input type="checkbox" checked={askScope} onChange={e => setAskScope(e.target.checked)} disabled={!page} /> About this page</label><button className="bp-primary" disabled={asking || !question.trim()}>{asking ? 'Reading the brain…' : 'Ask →'}</button></div>
    </form>
    {asking && <p className="bp-progress" role="status">Following the evidence. This can take a minute or two.</p>}
    {askError && <p className="bp-error" role="alert">{askError}</p>}
    {answer && <div className="bp-answer"><span className="bp-kind">{answer.scope}</span><h3>{answer.question}</h3><div className="bp-prose">{rich(answer.answer)}</div></div>}
    {!answer && !asking && <div className="bp-starters"><span className="bp-eyebrow">Try a real question</span>{['What have we learned about agent review loops?', 'Why do terminal dimensions go wrong?', 'How should we judge a game asset?'].map(q => <button key={q} onClick={() => { setQuestion(q); void ask(q) }}>{q}<span>↗</span></button>)}</div>}
  </section>

  const related = page && <aside className="bp-evidence" aria-label="Page references">
    <div className="bp-section-heading"><span>Follow the evidence</span><span>{page.related.filter(r => r.relation === 'links_to').length}</span></div>
    <p className="bp-muted">References from this page. Open one to inspect what it says.</p>
    {page.related.filter(r => r.relation === 'links_to').map(r => <button key={r.slug} onClick={() => open(r.slug)}><span className="bp-kind">{r.kind}</span><strong>{r.title || label(r.slug)}</strong><span>{r.summary}</span></button>)}
    {!page.related.some(r => r.relation === 'links_to') && <p className="bp-muted">This page has no outgoing references.</p>}
    {page.related.some(r => r.relation === 'linked_from') && <details><summary>Mentioned by {page.related.filter(r => r.relation === 'linked_from').length} pages</summary>{page.related.filter(r => r.relation === 'linked_from').map(r => <button key={r.slug} onClick={() => open(r.slug)}><span className="bp-kind">{r.kind}</span><strong>{r.title || label(r.slug)}</strong></button>)}</details>}
  </aside>

  const readerView = <div className="bp-reading-layout">
    <div className="bp-reader" ref={reader} onScroll={e => { if (nav.slug) scrolls.current.set(nav.slug, e.currentTarget.scrollTop) }}>
      {pageError ? <div className="bp-empty bp-error" role="alert"><h3>Couldn’t open this page</h3><p>{pageError}</p><button onClick={() => go({ slug: '' })}>Return to subjects</button></div> : !page ? <p className="bp-loading" role="status">Opening the page…</p> : <article>
        <div className="bp-page-meta"><span>{page.collection}</span><span> / </span><span>{page.kind}</span>{page.updated && <span className="bp-date">Updated {page.updated.slice(0, 10)}</span>}</div>
        <h1>{page.title}</h1>
        <p className="bp-deck">{page.summary}</p>
        <div className="bp-page-tools"><button onClick={() => { setAskOpen(true); setAskScope(true); setQuestion(`What are the strongest evidence and limitations behind “${page.title}”?`) }}>Ask about this page ↗</button><button onClick={() => { if (!navigator.clipboard) { setNotice('Copy the page address from your browser.'); return }; void navigator.clipboard.writeText(location.href).then(() => setNotice('Page link copied.')).catch(() => setNotice('Copy the page address from your browser.')) }}>Copy link</button></div>
        {notice && <p className="bp-notice" role="status">{notice}</p>}
        {page.kind === 'source' && <section className="bp-source-box"><div className="bp-section-heading"><span>Original material</span><button onClick={() => void readSource()} disabled={sourceBusy}>{source ? 'Reload source' : 'Read original →'}</button></div>
          {(page.source?.origin_path || page.source?.url) && <p className="bp-source-ref">{page.source.url || page.source.origin_path}</p>}
          {sourceBusy && <p role="status">Reading preserved bytes…</p>}{sourceError && <p className="bp-error" role="alert">{sourceError}</p>}
          {source && source.status !== 'ok' && <p>{source.status === 'external' ? 'This material lives outside the stored text collection.' : source.reason || source.message || `Source unavailable: ${source.status}`}{source.reference && <code>{source.reference}</code>}</p>}
          {source?.status === 'ok' && <><span className="bp-kind">Preserved source · {source.sha256 ? `SHA-256 ${source.sha256.slice(0, 12)}…` : 'original text'}</span><pre className="bp-source-text">{source.content}</pre>{source.truncated && <button disabled={sourceBusy} onClick={() => void readSource(true)}>Read more ↓</button>}</>}
        </section>}
        <div className="bp-prose">{rich(page.content.trimStart().replace(/^#\s+[^\n]+\n/, ''))}</div>
        <footer className="bp-page-footer">{page.slug} · {page.kind}<span>Explore the references to inspect the basis for a claim.</span></footer>
      </article>}
    </div>
    {related}
  </div>

  function ReadingRoom() {
    return <div className="bp-room">{isReading ? readerView : <div className="bp-home-scroll" ref={listing} onScroll={e => scrolls.current.set(listKey, e.currentTarget.scrollTop)}><div className="bp-home-inner"><div className="bp-hero"><span className="bp-eyebrow">The company brain</span><h1>What we know.<br /><span>And how we know it.</span></h1><p>Find the current understanding. Follow it back to the evidence.</p>{searchForm}</div><div className="bp-home-controls">{filters}</div>{errors}{resultHeading}{empty}{topicList(listed)}{!nav.q && <section className="bp-recent"><div className="bp-section-heading"><span>Recently dated pages</span><span>From page metadata</span></div>{topicList((catalog?.recent || []).filter(p => p.collection === nav.collection).slice(0, 5), true)}</section>}</div></div>}{askOpen && <div className="bp-ask-dock">{askPanel}</div>}</div>
  }
  function Explorer() {
    return <div className="bp-explorer"><aside className="bp-explorer-index"><h2>Browse the brain</h2>{searchForm}{filters}{errors}{resultHeading}<div className="bp-index-scroll" ref={listing} onScroll={e => scrolls.current.set(listKey, e.currentTarget.scrollTop)}>{empty}{topicList(listed, true)}</div></aside><div className="bp-explorer-content">{isReading ? readerView : <div className="bp-empty bp-explorer-welcome"><span className="bp-eyebrow">Start with a subject</span><h1>Keep the whole trail<br />within reach.</h1><p>Choose a page on the left. Its references will sit beside your reading.</p><button onClick={() => open(topics[0]?.slug || 'knowledge-organization')}>Open the first topic →</button></div>}</div>{askOpen && <div className="bp-ask-dock">{askPanel}</div>}</div>
  }
  function ResearchDesk() {
    return <div className="bp-research"><div className="bp-research-question">{askPanel}</div><div className="bp-research-reading">{isReading ? readerView : <div className="bp-research-home" ref={listing} onScroll={e => scrolls.current.set(listKey, e.currentTarget.scrollTop)}><span className="bp-eyebrow">Read alongside your question</span><h1>Keep the evidence open.</h1><p className="bp-muted">Follow a citation from an answer, or find a subject yourself.</p>{searchForm}{filters}{errors}{resultHeading}{empty}{topicList(listed, true)}</div>}</div></div>
  }
  return <section className={`brain-prototype bp-variant-${nav.variant}`} aria-label="Brain Library prototype">
    <header className="bp-header"><div className="bp-identity"><span className="bp-mark" aria-hidden="true">◈</span><strong>Library</strong><span className="bp-prototype-label">Prototype</span></div><nav aria-label="Reading navigation"><button onClick={() => history.state?.brainDepth ? history.back() : go({ slug: '', q: '' }, true)} disabled={!nav.slug && !nav.q && !history.state?.brainDepth} title="Previous page">← Back</button><button onClick={() => go({ slug: '', q: '' })}>Subjects</button>{page && <span className="bp-breadcrumb">/ {page.title}</span>}</nav><div className="bp-header-actions"><button onClick={() => { setAskOpen(v => !v) }}>Ask</button><button title="Refresh the brain catalog" onClick={() => { void loadCatalog(true) }}>Refresh</button><span className="bp-live">Live brain · read only</span></div></header>
    {nav.variant === 'A' ? ReadingRoom() : nav.variant === 'B' ? Explorer() : ResearchDesk()}
    <div className="bp-switcher" aria-label="Prototype layouts"><span className="bp-switcher-label">Compare layouts</span><button aria-label="Previous layout" onClick={() => go({ variant: variants[(variants.findIndex(v => v.key === nav.variant) + 2) % 3].key }, true)}>←</button><strong>{current.key} · {current.name}</strong><button aria-label="Next layout" onClick={() => go({ variant: variants[(variants.findIndex(v => v.key === nav.variant) + 1) % 3].key }, true)}>→</button></div>
  </section>
}

export default memo(BrainPrototype)
