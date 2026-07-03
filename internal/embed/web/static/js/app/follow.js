function inFollowScope(scope, path) {
	if (scope !== 'dir') return true
	var pathname = decodeURIComponent(window.location.pathname)
	if (!pathname.startsWith('/file/')) return true
	var current = pathname.slice('/file/'.length)
	if (!current.includes('/')) return true
	var dir = current.slice(0, current.lastIndexOf('/') + 1)
	return path.startsWith(dir)
}

function handleFollowKeydown(evt) {
	if (evt.key !== 'f' && evt.key !== 'F') return
	if (evt.ctrlKey || evt.metaKey || evt.altKey) return
	var t = evt.target
	if (t && (t.matches('input, textarea, select') || t.isContentEditable)) return
	if (!document.querySelector('[data-follow-toggle]')) return
	var store = Alpine.store('follow')
	if (store) store.toggle()
}

document.addEventListener('alpine:init', function() {
	Alpine.store('follow', {
		enabled: localStorage.getItem('stigmergic-follow') === 'true',
		paused: false,
		autoPause: localStorage.getItem('stigmergic-follow-autopause') === 'true',
		scope: localStorage.getItem('stigmergic-follow-scope') === 'dir' ? 'dir' : 'corpus',
		pendingPath: null,
		timer: null,
		selfNav: false,

		get active() {
			return this.enabled && !this.paused
		},

		label() {
			if (!this.enabled) return 'Follow: Off'
			return this.paused ? 'Follow: Paused' : 'Following'
		},

		toggle() {
			if (this.enabled && this.paused) {
				this.paused = false
				return
			}
			this.enabled = !this.enabled
			this.paused = false
			localStorage.setItem('stigmergic-follow', String(this.enabled))
		},

		setScope(scope) {
			this.scope = scope
			localStorage.setItem('stigmergic-follow-scope', scope)
		},

		// Disabling auto-pause while paused resumes immediately: with the
		// option off there is no UI state that explains a stranded pause.
		setAutoPause(value) {
			this.autoPause = value
			localStorage.setItem('stigmergic-follow-autopause', String(value))
			if (!value) this.paused = false
		},

		pause() {
			if (this.enabled && !this.paused) this.paused = true
		},

		// handleChange is called by the SSE dispatcher with the changed path.
		// Returns true when follow mode owns the response to this event, so
		// the caller skips its own content refresh. Navigation is debounced;
		// the newest event wins.
		handleChange(path) {
			if (!this.active || !path) return false
			if (!inFollowScope(this.scope, path)) return false
			this.pendingPath = path
			clearTimeout(this.timer)
			var store = this
			this.timer = setTimeout(function() {
				store.navigate(store.pendingPath)
			}, 150)
			return true
		},

		navigate(path) {
			var url = '/file/' + encodeURI(path)
			var current = decodeURIComponent(window.location.pathname)
			this.selfNav = true
			try {
				if (current === '/file/' + path) {
					htmx.ajax('GET', url, {target: '#content', swap: 'innerHTML'})
				} else {
					var source = document.querySelector('[data-follow-toggle]')
					if (!source) return
					htmx.ajax('GET', url, {source: source, target: '#content', swap: 'innerHTML'})
				}
			} finally {
				this.selfNav = false
			}
		}
	})
})

document.addEventListener('DOMContentLoaded', function() {
	document.body.addEventListener('htmx:beforeRequest', function(evt) {
		if (!window.Alpine) return
		var store = Alpine.store('follow')
		if (!store) return
		if (!store.autoPause) return
		if (store.selfNav || window.stigmergicProgrammaticNav) return
		var target = evt.detail.target
		if (target && target.id === 'content') store.pause()
	})

	window.addEventListener('popstate', function() {
		if (!window.Alpine) return
		var store = Alpine.store('follow')
		if (store && store.autoPause) store.pause()
	})

	document.addEventListener('keydown', handleFollowKeydown)
})
