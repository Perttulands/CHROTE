// CHROTE documentation: menu, search, copy, and where-you-are in the page outline.
// No framework and no network: the search index ships beside this file.
(function () {
  'use strict'

  var root = (function () {
    var link = document.querySelector('link[rel="stylesheet"][href$="assets/site.css"]')
    return link ? link.getAttribute('href').replace(/assets\/site\.css$/, '') : ''
  })()

  // ---- Toast: the confirmation badge in the bottom-centre slot. ----------
  var toast = document.querySelector('.toast')
  var toastTimer = null
  function announce(kind, message, failed) {
    if (!toast) return
    toast.innerHTML = ''
    var cap = document.createElement('span')
    cap.className = 'cap' + (failed ? ' fail' : '')
    cap.textContent = kind
    toast.appendChild(cap)
    toast.appendChild(document.createTextNode(message))
    toast.classList.add('on')
    clearTimeout(toastTimer)
    toastTimer = setTimeout(function () { toast.classList.remove('on') }, 1920)
  }

  // ---- Menu (narrow screens) ----------------------------------------------
  var toggle = document.querySelector('.menu-toggle')
  var sidebar = document.getElementById('sidebar')
  function setMenu(open) {
    document.body.classList.toggle('nav-open', open)
    if (toggle) {
      toggle.setAttribute('aria-expanded', String(open))
      toggle.textContent = open ? 'Close' : 'Menu'
    }
    if (open && sidebar) {
      var here = sidebar.querySelector('[aria-current="page"]') || sidebar.querySelector('a')
      if (here) here.focus()
    }
  }
  if (toggle) toggle.addEventListener('click', function () { setMenu(!document.body.classList.contains('nav-open')) })
  document.addEventListener('click', function (event) {
    if (!document.body.classList.contains('nav-open')) return
    if (sidebar.contains(event.target) || toggle.contains(event.target)) return
    setMenu(false)
  })
  var current = sidebar && sidebar.querySelector('[aria-current="page"]')
  if (current && window.matchMedia('(min-width: 961px)').matches) {
    // Scroll the sidebar alone, so the page itself opens at its top.
    var top = current.getBoundingClientRect().top - sidebar.getBoundingClientRect().top
    if (top > sidebar.clientHeight * 0.7) sidebar.scrollTop = top - sidebar.clientHeight / 2
  }

  // ---- Copy ---------------------------------------------------------------
  document.addEventListener('click', function (event) {
    var button = event.target.closest('[data-copy]')
    if (!button) return
    var code = button.closest('.code').querySelector('pre code')
    var text = code.innerText.replace(/\n$/, '')
    var done = function () { announce('Copied', code.innerText.split('\n').length + ' lines') }
    var fail = function () { announce('Copy', 'The browser refused the clipboard', true) }
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(done, fail)
    } else {
      var area = document.createElement('textarea')
      area.value = text
      area.setAttribute('readonly', '')
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.appendChild(area)
      area.select()
      try { document.execCommand('copy') ? done() : fail() } catch (e) { fail() }
      document.body.removeChild(area)
    }
  })

  // ---- Where you are in the outline -----------------------------------------
  var tocLinks = Array.prototype.slice.call(document.querySelectorAll('.toc a'))
  if (tocLinks.length && 'IntersectionObserver' in window) {
    var targets = tocLinks.map(function (a) { return document.getElementById(decodeURIComponent(a.hash.slice(1))) })
    var visible = new Set()
    var mark = function () {
      var index = -1
      for (var i = 0; i < targets.length; i++) {
        if (!targets[i]) continue
        if (targets[i].getBoundingClientRect().top < window.innerHeight * 0.3) index = i
      }
      tocLinks.forEach(function (a, i) { a.classList.toggle('here', i === Math.max(index, 0)) })
    }
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) { e.isIntersecting ? visible.add(e.target) : visible.delete(e.target) })
      mark()
    }, { rootMargin: '0px 0px -60% 0px' })
    targets.forEach(function (t) { if (t) observer.observe(t) })
    window.addEventListener('scroll', mark, { passive: true })
    mark()
  }

  // ---- Search ---------------------------------------------------------------
  var input = document.getElementById('search-input')
  var list = document.getElementById('search-results')
  var index = window.CHROTE_SEARCH || []
  var results = []
  var active = -1

  function escapeHtml(s) {
    return s.replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c] })
  }
  function highlight(text, terms) {
    var out = escapeHtml(text)
    terms.forEach(function (t) {
      if (t.length < 2) return
      out = out.replace(new RegExp('(' + t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')', 'ig'), '<mark>$1</mark>')
    })
    return out
  }
  function snippet(text, terms) {
    var lower = text.toLowerCase()
    var at = -1
    for (var i = 0; i < terms.length && at < 0; i++) at = lower.indexOf(terms[i])
    if (at < 0) return ''
    var start = Math.max(0, at - 50)
    return (start > 0 ? '…' : '') + text.slice(start, start + 150).trim() + '…'
  }

  function search(query) {
    var terms = query.toLowerCase().split(/\s+/).filter(Boolean)
    if (!terms.length) return []
    var found = []
    index.forEach(function (page) {
      var title = page.title.toLowerCase()
      var text = page.text.toLowerCase()
      var summary = (page.summary || '').toLowerCase()
      var guide = page.section !== 'Source documents' && page.section !== 'Decisions'
      var pageScore = 0
      var all = terms.every(function (t) {
        var s = 0
        if (title.indexOf(t) >= 0) s += 12
        if (summary.indexOf(t) >= 0) s += 4
        if (text.indexOf(t) >= 0) s += 1 + Math.min(4, text.split(t).length - 1) * 0.5
        page.headings.forEach(function (h) { if (h.text.toLowerCase().indexOf(t) >= 0) s += 3 })
        pageScore += s
        return s > 0
      })
      if (!all) return
      if (guide) pageScore *= 1.5
      found.push({ url: page.url, title: page.title, where: page.section, score: pageScore, text: snippet(page.text, terms) })
      page.headings.forEach(function (h) {
        var ht = h.text.toLowerCase()
        if (terms.every(function (t) { return ht.indexOf(t) >= 0 })) {
          found.push({ url: page.url + '#' + h.id, title: h.text, where: page.title, score: pageScore * 0.9 + 6, text: '' })
        }
      })
    })
    found.sort(function (a, b) { return b.score - a.score })
    return found.slice(0, 12)
  }

  function render(query) {
    var terms = query.toLowerCase().split(/\s+/).filter(Boolean)
    results = search(query)
    active = results.length ? 0 : -1
    list.innerHTML = ''
    if (!query.trim()) { close(); return }
    if (!results.length) {
      list.innerHTML = '<li class="empty" role="presentation">Nothing in the docs says “' + escapeHtml(query) + '”.</li>'
    }
    results.forEach(function (r, i) {
      var li = document.createElement('li')
      li.id = 'result-' + i
      li.setAttribute('role', 'option')
      li.innerHTML = '<a href="' + root + r.url + '" tabindex="-1"><span class="r-title">' + highlight(r.title, terms) +
        '</span><span class="r-where">' + escapeHtml(r.where) + '</span>' +
        (r.text ? '<span class="r-text">' + highlight(r.text, terms) + '</span>' : '') + '</a>'
      list.appendChild(li)
    })
    list.hidden = false
    input.setAttribute('aria-expanded', 'true')
    markActive()
  }
  function markActive() {
    Array.prototype.forEach.call(list.children, function (li, i) {
      if (li.getAttribute('role') !== 'option') return
      li.setAttribute('aria-selected', String(i === active))
      if (i === active) {
        var a = li.offsetTop, b = a + li.offsetHeight
        if (a < list.scrollTop) list.scrollTop = a
        else if (b > list.scrollTop + list.clientHeight) list.scrollTop = b - list.clientHeight
      }
    })
    if (active >= 0) input.setAttribute('aria-activedescendant', 'result-' + active)
    else input.removeAttribute('aria-activedescendant')
  }
  function close() {
    list.hidden = true
    input.setAttribute('aria-expanded', 'false')
    input.removeAttribute('aria-activedescendant')
  }

  if (input) {
    input.addEventListener('input', function () { render(input.value) })
    input.addEventListener('focus', function () { if (input.value.trim()) render(input.value) })
    input.addEventListener('keydown', function (event) {
      if (event.key === 'ArrowDown' && results.length) { active = (active + 1) % results.length; markActive(); event.preventDefault() }
      else if (event.key === 'ArrowUp' && results.length) { active = (active - 1 + results.length) % results.length; markActive(); event.preventDefault() }
      else if (event.key === 'Enter' && active >= 0) { window.location.href = root + results[active].url; close() }
      else if (event.key === 'Escape') {
        if (!list.hidden) close()
        else { input.value = ''; input.blur() }
        event.preventDefault()
      }
    })
    document.addEventListener('click', function (event) {
      if (!event.target.closest('.search')) close()
    })
  }

  document.addEventListener('keydown', function (event) {
    var typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName) || document.activeElement.isContentEditable
    if (event.key === '/' && !typing && !event.altKey && !event.ctrlKey && !event.metaKey) {
      event.preventDefault()
      input.focus()
      input.select()
    } else if (event.key === 'Escape' && document.body.classList.contains('nav-open')) {
      setMenu(false)
      toggle.focus()
    }
  })
})()
