function scrollProgress() {
	return {
		progress: 0,
		container: null,
		init() {
			this.container = document.getElementById('content')
			if (!this.container) return
			this.updateProgress()
			this.container.addEventListener('scroll', () => {
				this.updateProgress()
			}, { passive: true })
			document.body.addEventListener('htmx:afterSwap', () => {
				this.updateProgress()
			})
		},
		updateProgress() {
			var el = this.container
			var height = el.scrollHeight - el.clientHeight
			this.progress = height > 0 ? (el.scrollTop / height) * 100 : 0
		}
	}
}

function copyPath(path) {
	return {
		copied: false,
		path: path,
		copy() {
			navigator.clipboard.writeText(this.path).then(() => {
				this.copied = true
				setTimeout(() => {
					this.copied = false
				}, 2000)
			})
		}
	}
}

function rawToggle() {
	return {
		showRaw: false,
		showLineNumbers: false,
		lines: [],
		init() {
			this.loadRawContent()
			document.body.addEventListener('htmx:afterSwap', () => {
				this.showRaw = false
				this.showLineNumbers = false
				this.loadRawContent()
			})
		},
		loadRawContent() {
			var el = document.getElementById('raw-content-data')
			if (el) {
				try {
					var content = JSON.parse(el.textContent)
					this.lines = content.split('\n')
				} catch (e) {
					this.lines = []
				}
			}
		},
		toggle() {
			this.showRaw = !this.showRaw
		},
		get buttonText() {
			return this.showRaw ? 'Rendered' : 'Source'
		}
	}
}

function themeConfig() {
	var el = document.getElementById('theme-config')
	if (!el) return null
	try {
		return JSON.parse(el.textContent)
	} catch (e) {
		console.error('unparseable theme-config', e)
		return null
	}
}

// applyTheme swaps the palette by attribute (the scoped variable and chroma
// blocks are already on the page), persists the choice, and re-themes
// mermaid by re-rendering every diagram from its stashed source.
function applyTheme(name) {
	document.documentElement.setAttribute('data-theme', name)
	try {
		localStorage.setItem('stigmergic-theme', name)
	} catch (e) {
		console.warn('theme preference not persisted', e)
	}
	var cfg = themeConfig()
	if (cfg && window.mermaid) {
		mermaid.initialize({ startOnLoad: false, theme: cfg.mermaid[name] || 'dark' })
		renderMermaidIn(document.body)
	}
}

function cycleTheme() {
	var cfg = themeConfig()
	if (!cfg || !cfg.order.length) return
	var active = document.documentElement.getAttribute('data-theme') || cfg.boot
	var idx = cfg.order.indexOf(active)
	applyTheme(cfg.order[(idx + 1) % cfg.order.length])
}

function handleThemeKeydown(evt) {
	if (evt.key !== 't' && evt.key !== 'T') return
	if (evt.ctrlKey || evt.metaKey || evt.altKey) return
	var t = evt.target
	if (t && (t.matches('input, textarea, select') || t.isContentEditable)) return
	cycleTheme()
}

function helpOverlay() {
	return {
		open: false,
		toggle() {
			this.open = !this.open
		},
		close() {
			this.open = false
		},
		onKeydown(evt) {
			if (evt.key === 'Escape') {
				this.close()
				return
			}
			if (evt.key !== '?') return
			var t = evt.target
			if (t && (t.matches('input, textarea, select') || t.isContentEditable)) return
			evt.preventDefault()
			this.toggle()
		}
	}
}
