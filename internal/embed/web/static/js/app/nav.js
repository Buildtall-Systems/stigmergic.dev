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
