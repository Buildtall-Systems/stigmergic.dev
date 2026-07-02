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
	if (window.location.pathname === '/') {
		initKeyboardNav();
	}
	document.addEventListener('keydown', handleNavKeydown);
	document.addEventListener('keydown', handleSourceKeydown);

	document.body.addEventListener('htmx:afterSwap', function(evt) {
		renderMath(evt.detail.target);
		renderWiremd(evt.detail.target);
		initCodeCopyButtons();
		renderMermaidIn(evt.detail.target);
		if (evt.detail.target.id === 'content') {
			evt.detail.target.scrollTop = 0;
			syncCurrentFile();
			if (window.location.pathname === '/') {
				initKeyboardNav();
			}
		}
		if (evt.detail.target.id === 'sidebar') {
			syncCurrentFile();
		}
	});

	document.body.addEventListener('sse:message', function(evt) {
		var data = evt.detail ? evt.detail.data : null;
		if (data === 'index-ready') {
			document.body.dataset.indexReady = 'true';
			document.body.dispatchEvent(new CustomEvent('indexReady'));
			return;
		}
		htmx.ajax('GET', window.location.pathname, {target: '#content', swap: 'innerHTML'});
		htmx.ajax('GET', '/partial/sidebar', {target: '#sidebar', swap: 'innerHTML'});
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
