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
