function toggleDirectory(element) {
	const isExpanded = element.getAttribute('data-expanded') === 'true';
	const parent = element.parentElement;
	const childrenDiv = parent.querySelector('.directory-children');
	const chevron = element.querySelector('.chevron-btn svg');

	if (isExpanded) {
		childrenDiv.style.display = 'none';
		chevron.classList.remove('rotate-90');
		element.setAttribute('data-expanded', 'false');
	} else {
		childrenDiv.style.display = 'block';
		chevron.classList.add('rotate-90');
		element.setAttribute('data-expanded', 'true');
	}
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

function syncCurrentFile() {
	var pathname = decodeURIComponent(window.location.pathname)
	document.querySelectorAll('#sidebar [data-path]').forEach(function(item) {
		var isCurrent = '/file/' + item.getAttribute('data-path') === pathname
		item.classList.toggle('tree-item-current', isCurrent)
		if (isCurrent) {
			expandAncestors(item)
		}
	})
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

function handleOutlineClick(evt) {
	var link = evt.target.closest('#outline [data-outline-target]')
	if (!link) return
	var heading = document.getElementById(link.getAttribute('data-outline-target'))
	if (!heading) return
	evt.preventDefault()
	heading.scrollIntoView({behavior: 'smooth', block: 'start'})
	setActiveOutlineLink(heading.id)
}

function expandAncestors(item) {
	var children = item.closest('.directory-children')
	while (children) {
		children.style.display = 'block'
		var wrapper = children.parentElement
		var toggle = wrapper.querySelector('[data-expanded]')
		if (toggle) {
			toggle.setAttribute('data-expanded', 'true')
			var chevron = toggle.querySelector('.chevron-btn svg')
			if (chevron) {
				chevron.classList.add('rotate-90')
			}
		}
		children = wrapper.closest('.directory-children')
	}
}
