// treeChildrenURL is where an unmaterialized directory's rows come from. Each
// segment is encoded separately so the separators survive. A directory path
// is relative to its own source, so the row names the source it came from
// and the server reads the same tree the row was drawn from.
//
// A source's root is the empty remainder, never the "." its own tree calls
// it: a lone "." is cleaned out of a URL path and redirected away before it
// reaches a handler, so it is dropped here instead of bouncing twice.
function treeChildrenURL(path, mount) {
	var segments = path === '.' ? '' : path.split('/').map(encodeURIComponent).join('/')
	var url = '/partial/tree/' + segments
	if (mount) url += '?mount=' + encodeURIComponent(mount)
	return url
}

// loadDirectoryChildren fills a placeholder container, once. Resolves
// immediately when the rows are already present and returns the in-flight
// request when one is, so callers can await it unconditionally and a double
// click cannot issue two requests. htmx.ajax rather than fetch: the rows carry
// hx-get and must be processed, or clicking one would reload the whole page
// instead of swapping #content.
function loadDirectoryChildren(container) {
	if (container.dataset.loaded === 'true') return Promise.resolve()
	if (container.stigmergicPending) return container.stigmergicPending
	var path = container.dataset.childrenPath
	if (!path) return Promise.resolve()

	var pending = htmx.ajax('GET', treeChildrenURL(path, container.dataset.mount), {target: container, swap: 'innerHTML'})
		.then(function() {
			container.dataset.loaded = 'true'
		})
		.catch(function(err) {
			console.error('failed to load tree children', path, err)
		})
		.finally(function() {
			container.stigmergicPending = null
		})
	container.stigmergicPending = pending
	return pending
}

// expandContainer opens one directory's container and syncs the row above it.
function expandContainer(container) {
	container.style.display = 'block'
	var wrapper = container.parentElement
	if (!wrapper) return
	var toggle = wrapper.querySelector('[data-expanded]')
	if (!toggle) return
	toggle.setAttribute('data-expanded', 'true')
	var chevron = toggle.querySelector('.chevron-btn svg')
	if (chevron) chevron.classList.add('rotate-90')
}

function toggleDirectory(element) {
	const isExpanded = element.getAttribute('data-expanded') === 'true';
	const parent = element.parentElement;
	const childrenDiv = parent.querySelector('.directory-children');
	const chevron = element.querySelector('.chevron-btn svg');

	if (isExpanded) {
		childrenDiv.style.display = 'none';
		chevron.classList.remove('rotate-90');
		element.setAttribute('data-expanded', 'false');
		return;
	}
	childrenDiv.style.display = 'block';
	chevron.classList.add('rotate-90');
	element.setAttribute('data-expanded', 'true');
	loadDirectoryChildren(childrenDiv);
}

function initKeyboardNav() {
	document.querySelectorAll('[data-nav-item]').forEach(function(item) {
		item.style.backgroundColor = ''
	})
	var firstItem = document.querySelector('[data-nav-item]')
	if (firstItem) {
		firstItem.focus()
	}
}

function handleNavKeydown(evt) {
	if (evt.key !== 'ArrowDown' && evt.key !== 'ArrowUp') return
	var active = document.activeElement
	if (!active || !active.hasAttribute('data-nav-item')) return

	var items = Array.from(document.querySelectorAll('[data-nav-item]'))
	var idx = items.indexOf(active)
	if (idx === -1) return

	evt.preventDefault()
	var nextIdx
	if (evt.key === 'ArrowDown') {
		nextIdx = idx + 1 < items.length ? idx + 1 : 0
	} else {
		nextIdx = idx - 1 >= 0 ? idx - 1 : items.length - 1
	}
	items[nextIdx].focus()
}

// routeParts splits a pathname into the route prefix of the source serving
// it and the path inside that source, or null on any page that is not a
// document view. A vault's prefix carries its owner and its name, so it is
// three segments deep where the local tree's is one.
function routeParts(pathname) {
	if (pathname.indexOf('/file/') === 0) {
		return {mount: '/file/', path: pathname.slice('/file/'.length)}
	}
	if (pathname.indexOf('/vault/') === 0) {
		var parts = pathname.split('/')
		if (parts.length < 5) return null
		return {mount: '/vault/' + parts[2] + '/' + parts[3] + '/', path: parts.slice(4).join('/')}
	}
	return null
}

// currentFilePath is the path of the document on screen inside the primary
// source, or '' anywhere else. Deliberately blank for a vault document: the
// only caller hands it to the sidebar partial, which opens the local tree,
// and a vault's path names nothing that tree holds.
function currentFilePath() {
	var route = routeParts(decodeURIComponent(window.location.pathname))
	if (!route || route.mount !== '/file/') return ''
	return route.path
}

// markCurrentFile highlights the row for the document on screen and reports
// whether it found one. A miss means the row lives in a subtree that has not
// been fetched.
//
// A row is matched on its whole route rather than its path, because two
// mounted sources can hold the same path and only one of them is on screen.
function markCurrentFile() {
	var pathname = decodeURIComponent(window.location.pathname)
	var found = false
	document.querySelectorAll('#sidebar [data-path]').forEach(function(item) {
		var mount = item.getAttribute('data-mount') || '/file/'
		var isCurrent = mount + item.getAttribute('data-path') === pathname
		item.classList.toggle('tree-item-current', isCurrent)
		if (isCurrent) {
			found = true
			expandAncestors(item)
		}
	})
	return found
}

// attrValue escapes a path for use inside a double-quoted attribute selector.
// Tree paths are arbitrary filenames, so quotes and backslashes are possible.
function attrValue(s) {
	return s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

// revealPath materializes the rows leading to a file inside one mount and
// opens them. The directories load in sequence because a nested placeholder
// does not exist in the DOM until its parent's rows have arrived.
//
// The chain starts at the source's own root, ".", which is the container a
// vault row holds and the local tree has none of: the sidebar ships the
// primary tree's rows already, so that first step finds nothing and costs
// nothing. Containers are matched on mount as well as path, because two
// sources can hold a directory of the same name.
function revealPath(mount, filePath) {
	var parts = filePath.split('/')
	var chain = ['.']
	var acc = ''
	for (var i = 0; i < parts.length - 1; i++) {
		acc = acc ? acc + '/' + parts[i] : parts[i]
		chain.push(acc)
	}
	return chain.reduce(function(prev, dir) {
		return prev.then(function() {
			var selector = '#sidebar .directory-children[data-children-path="' + attrValue(dir) + '"]' +
				'[data-mount="' + attrValue(mount) + '"]'
			var container = document.querySelector(selector)
			if (!container) return
			expandContainer(container)
			return loadDirectoryChildren(container)
		})
	}, Promise.resolve())
}

// syncCurrentFile highlights the current document's row, fetching the
// directories on the way to it when the tree has not materialized that far.
// The server expands the ancestor chain on a full page load, so this only
// fetches after a swap that changed #content alone.
function syncCurrentFile() {
	if (markCurrentFile()) return
	var route = routeParts(decodeURIComponent(window.location.pathname))
	if (!route || !route.path) return
	revealPath(route.mount, route.path).then(markCurrentFile)
}

var outlineObserver = null

function setActiveOutlineLink(id) {
	document.querySelectorAll('#outline [data-outline-target]').forEach(function(link) {
		link.classList.toggle('outline-link-active', link.getAttribute('data-outline-target') === id)
	})
}

// initScrollspy rebuilds the outline observer for the current document.
// Idempotent: called after every #content or #outline swap; tears down the
// previous observer first. #content is the scroll container, so it is the
// observer root — not the viewport.
function initScrollspy() {
	if (outlineObserver) {
		outlineObserver.disconnect()
		outlineObserver = null
	}
	var links = document.querySelectorAll('#outline [data-outline-target]')
	if (!links.length) return
	var content = document.getElementById('content')
	if (!content) return

	var headings = []
	links.forEach(function(link) {
		var heading = document.getElementById(link.getAttribute('data-outline-target'))
		if (heading) headings.push(heading)
	})
	if (!headings.length) return

	var visible = new Set()
	outlineObserver = new IntersectionObserver(function(entries) {
		entries.forEach(function(entry) {
			if (entry.isIntersecting) {
				visible.add(entry.target.id)
			} else {
				visible.delete(entry.target.id)
			}
		})
		var active = null
		for (var i = 0; i < headings.length; i++) {
			if (visible.has(headings[i].id)) {
				active = headings[i].id
				break
			}
		}
		if (!active) {
			// Between sections: the section being read is the one whose
			// heading most recently scrolled off the top.
			var contentTop = content.getBoundingClientRect().top
			for (var j = headings.length - 1; j >= 0; j--) {
				if (headings[j].getBoundingClientRect().top < contentTop) {
					active = headings[j].id
					break
				}
			}
		}
		if (active) setActiveOutlineLink(active)
	}, {root: content, rootMargin: '0px 0px -60% 0px'})

	headings.forEach(function(h) {
		outlineObserver.observe(h)
	})
}

// outlineHeadings resolves the outline rail's links to their heading elements
// in document order — the same list the scrollspy observes.
function outlineHeadings() {
	var headings = []
	document.querySelectorAll('#outline [data-outline-target]').forEach(function(link) {
		var heading = document.getElementById(link.getAttribute('data-outline-target'))
		if (heading) headings.push(heading)
	})
	return headings
}

// nearestBlock finds the closest element past the top of the reading pane in
// the given direction, or null at either end of the document. The 1px
// tolerance keeps an element parked exactly at the top from matching itself.
function nearestBlock(elements, contentTop, direction) {
	if (direction > 0) {
		for (var i = 0; i < elements.length; i++) {
			if (elements[i].getBoundingClientRect().top > contentTop + 1) {
				return elements[i]
			}
		}
		return null
	}
	for (var j = elements.length - 1; j >= 0; j--) {
		if (elements[j].getBoundingClientRect().top < contentTop - 1) {
			return elements[j]
		}
	}
	return null
}

// jumpToSection scrolls to the nearest heading beyond the top of the reading
// pane in the given direction. Geometry-based rather than active-link-based:
// backward from mid-section lands on the current section's heading first,
// matching media-player prev semantics.
function jumpToSection(direction) {
	var content = document.getElementById('content')
	if (!content) return
	var headings = outlineHeadings()
	if (!headings.length) return

	var target = nearestBlock(headings, content.getBoundingClientRect().top, direction)
	if (!target) return
	target.scrollIntoView({behavior: 'smooth', block: 'start'})
	setActiveOutlineLink(target.id)
}

// documentBlocks lists the rendered document's top-level blocks: paragraphs,
// lists, code fences, tables, and headings alike. Elements hidden by the
// source toggle report a zero rect, so they are filtered out here rather than
// mis-measured by the geometry walk.
function documentBlocks() {
	var article = document.querySelector('#content article')
	if (!article) return []
	return Array.from(article.children).filter(function(el) {
		return el.offsetParent !== null
	})
}

// jumpToParagraph is jumpToSection at block granularity. No outline update:
// the scrollspy observer keeps the active link current as headings cross.
function jumpToParagraph(direction) {
	var content = document.getElementById('content')
	if (!content) return
	var blocks = documentBlocks()
	if (!blocks.length) return

	var target = nearestBlock(blocks, content.getBoundingClientRect().top, direction)
	if (!target) return
	target.scrollIntoView({behavior: 'smooth', block: 'start'})
}

function handleSectionKeydown(evt) {
	if (evt.key !== 'n' && evt.key !== 'N' && evt.key !== 'p' && evt.key !== 'P') return
	if (evt.ctrlKey || evt.metaKey || evt.altKey) return
	var t = evt.target
	if (t && (t.matches('input, textarea, select') || t.isContentEditable)) return
	jumpToSection(evt.key === 'n' || evt.key === 'N' ? 1 : -1)
}

function handleParagraphKeydown(evt) {
	if (evt.key !== 'j' && evt.key !== 'J' && evt.key !== 'k' && evt.key !== 'K') return
	if (evt.ctrlKey || evt.metaKey || evt.altKey) return
	var t = evt.target
	if (t && (t.matches('input, textarea, select') || t.isContentEditable)) return
	jumpToParagraph(evt.key === 'j' || evt.key === 'J' ? 1 : -1)
}

function handleOutlineClick(evt) {
	var link = evt.target.closest('#outline [data-outline-target]')
	if (!link) return
	var heading = document.getElementById(link.getAttribute('data-outline-target'))
	if (!heading) return
	evt.preventDefault()
	heading.scrollIntoView({behavior: 'smooth', block: 'start'})
	setActiveOutlineLink(heading.id)
}

// expandAncestors opens every container above a row already in the DOM. The
// path-driven counterpart is revealPath, which is what runs when the row is
// not there yet.
function expandAncestors(item) {
	var children = item.closest('.directory-children')
	while (children) {
		expandContainer(children)
		var wrapper = children.parentElement
		if (!wrapper) return
		children = wrapper.closest('.directory-children')
	}
}
