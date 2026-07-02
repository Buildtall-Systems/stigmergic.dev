// parseSSEEvent decodes the JSON envelope pushed by the server. Bare legacy
// strings (a stale cached page against a newer server, or vice versa) degrade
// to the old blind-refresh behavior instead of erroring.
function parseSSEEvent(data) {
	if (typeof data === 'string' && data.charAt(0) === '{') {
		try {
			return JSON.parse(data)
		} catch (err) {
			console.error('unparseable SSE payload', data, err)
		}
	}
	if (data === 'index-ready') return {type: 'index-ready'}
	return {type: 'reload', path: ''}
}

// contentShowsPath reports whether the reading pane is displaying the changed
// path: the file itself, a directory listing on its ancestor chain, or home.
// An empty path means "refresh regardless" (gitignore toggle, legacy reload).
function contentShowsPath(path) {
	if (!path) return true
	var pathname = decodeURIComponent(window.location.pathname)
	if (pathname === '/') return true
	if (!pathname.startsWith('/file/')) return false
	var current = pathname.slice('/file/'.length)
	return current === path || path.startsWith(current + '/')
}

// refreshPane refetches without a history push, flagged so the follow store
// does not mistake it for user-initiated navigation.
function refreshPane(url, target) {
	window.stigmergicProgrammaticNav = true
	try {
		htmx.ajax('GET', url, {target: target, swap: 'innerHTML'})
	} finally {
		window.stigmergicProgrammaticNav = false
	}
}

function handleSourceKeydown(evt) {
	if (evt.key !== 's' && evt.key !== 'S') return
	if (evt.ctrlKey || evt.metaKey || evt.altKey) return
	var t = evt.target
	if (t && (t.matches('input, textarea, select') || t.isContentEditable)) return
	window.dispatchEvent(new CustomEvent('toggle-source'))
}

document.addEventListener('DOMContentLoaded', function() {
	renderMath(document.body);
	renderWiremd(document.body);
	initCodeCopyButtons();
	syncCurrentFile();
	initScrollspy();
	if (window.location.pathname === '/') {
		initKeyboardNav();
	}
	document.addEventListener('keydown', handleNavKeydown);
	document.addEventListener('keydown', handleSourceKeydown);
	document.addEventListener('click', handleOutlineClick);

	document.body.addEventListener('htmx:afterSwap', function(evt) {
		renderMath(evt.detail.target);
		renderWiremd(evt.detail.target);
		initCodeCopyButtons();
		renderMermaidIn(evt.detail.target);
		if (evt.detail.target.id === 'content') {
			evt.detail.target.scrollTop = 0;
			syncCurrentFile();
			initScrollspy();
			if (window.location.pathname === '/') {
				initKeyboardNav();
			}
		}
		if (evt.detail.target.id === 'sidebar') {
			syncCurrentFile();
		}
	});

	// The outline rail arrives as an out-of-band fragment; htmx may process
	// it before or after the main #content swap, so both paths re-init.
	document.body.addEventListener('htmx:oobAfterSwap', function(evt) {
		if (evt.detail.target.id === 'outline') {
			initScrollspy();
		}
	});

	document.body.addEventListener('sse:message', function(evt) {
		var msg = parseSSEEvent(evt.detail ? evt.detail.data : null);
		if (msg.type === 'index-ready') {
			document.body.dataset.indexReady = 'true';
			document.body.dispatchEvent(new CustomEvent('indexReady'));
			return;
		}
		refreshPane('/partial/sidebar', '#sidebar');
		var follow = window.Alpine ? Alpine.store('follow') : null;
		if (follow && follow.handleChange(msg.path)) return;
		if (contentShowsPath(msg.path)) {
			refreshPane(window.location.pathname, '#content');
		}
	});

	window.addEventListener('pageshow', function(evt) {
		if (evt.persisted) {
			syncCurrentFile();
			if (window.location.pathname === '/') {
				initKeyboardNav();
			}
		}
	});

	document.body.addEventListener('htmx:historyRestore', function() {
		syncCurrentFile();
		if (window.location.pathname === '/') {
			initKeyboardNav();
		}
	});
});
