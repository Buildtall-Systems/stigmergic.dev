function renderMath(element) {
	renderMathInElement(element, {
		delimiters: [
			{left: '$$', right: '$$', display: true},
			{left: '$', right: '$', display: false}
		],
		throwOnError: false
	});
}

function renderWiremd(element) {
	element.querySelectorAll('pre.wiremd').forEach(function(block) {
		if (block.getAttribute('data-processed') === 'true') return
		try {
			var source = block.textContent
			var result = wiremd.render(source, 'sketch')
			if (result.css && !document.getElementById('wiremd-styles')) {
				var style = document.createElement('style')
				style.id = 'wiremd-styles'
				style.textContent = result.css
				document.head.appendChild(style)
			}
			var div = document.createElement('div')
			div.className = 'wiremd-rendered wmd-root wmd-sketch'
			div.innerHTML = result.html
			block.parentNode.replaceChild(div, block)
		} catch (e) {
			console.warn('wiremd render failed:', e.message)
			block.setAttribute('data-processed', 'true')
		}
	})
}

function initCodeCopyButtons() {
	document.querySelectorAll('pre').forEach(function(pre) {
		if (pre.classList.contains('mermaid')) return
		if (pre.classList.contains('wiremd')) return
		if (pre.querySelector('.copy-btn')) return

		var wrapper = document.createElement('div')
		wrapper.className = 'code-block-wrapper relative group'
		pre.parentNode.insertBefore(wrapper, pre)
		wrapper.appendChild(pre)

		var btn = document.createElement('button')
		btn.className = 'copy-btn absolute top-2 right-2 px-2 py-1 text-xs rounded-sm opacity-0 group-hover:opacity-100 transition-opacity'
		btn.style.cssText = 'background-color: var(--bg-alt-color); border: 1px solid var(--border-color); color: var(--comment-color);'
		btn.textContent = 'Copy'
		btn.onclick = function() {
			var code = pre.querySelector('code') || pre
			navigator.clipboard.writeText(code.textContent).then(function() {
				btn.textContent = 'Copied!'
				btn.style.color = 'var(--green-color)'
				setTimeout(function() {
					btn.textContent = 'Copy'
					btn.style.color = 'var(--comment-color)'
				}, 2000)
			})
		}
		wrapper.appendChild(btn)
	})
}

function renderMermaidIn(target) {
	target.querySelectorAll('pre.mermaid').forEach(function(node, i) {
		var id = 'mermaid-swap-' + Date.now() + '-' + i;
		mermaid.render(id, node.textContent).then(function(result) {
			node.innerHTML = result.svg;
			node.setAttribute('data-processed', 'true');
		});
	})
}
