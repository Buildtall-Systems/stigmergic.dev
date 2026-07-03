function commandPalette() {
	return {
		open: false,
		query: '',
		allFiles: [],
		respectGitignore: true,
		commands: [],
		results: [],
		displayGroups: [],
		contentResults: [],
		contentTruncated: false,
		selectedIndex: 0,
		indexReady: document.body.dataset.indexReady === 'true',
		gitignoreEnabled: document.body.dataset.gitignoreEnabled === 'true',

		buildCommands() {
			const commands = [
				{ id: 'cmd:home', type: 'command', name: 'Home', description: 'Go to index page', action: () => window.location.href = '/' }
			];
			if (this.gitignoreEnabled) {
				commands.push({
					id: 'cmd:gitignore',
					type: 'command',
					name: 'Toggle Gitignore',
					description: this.respectGitignore ? 'Currently respecting .gitignore (click to show ignored files)' : 'Currently showing all files (click to respect .gitignore)',
					action: () => this.toggleGitignore()
				});
			}
			return commands;
		},

		async toggleGitignore() {
			try {
				const res = await fetch('/api/gitignore/toggle', { method: 'POST' });
				const data = await res.json();
				this.respectGitignore = data.respectGitignore;
				this.commands = this.buildCommands();
				this.rebuildIndex();
			} catch (err) {
				console.error('Failed to toggle gitignore:', err);
			}
		},

		async loadGitignoreStatus() {
			try {
				const res = await fetch('/api/gitignore');
				const data = await res.json();
				this.respectGitignore = data.respectGitignore;
				this.commands = this.buildCommands();
				this.rebuildIndex();
			} catch (err) {
				console.error('Failed to load gitignore status:', err);
			}
		},

		getIcon(type) {
			return this._iconCache[type];
		},

		getReturnIcon() {
			return this._iconCache.return;
		},

		init() {
			this._iconCache = {
				command: document.getElementById('icon-command').innerHTML,
				file: document.getElementById('icon-file').innerHTML,
				content: document.getElementById('icon-content').innerHTML,
				return: document.getElementById('icon-return').innerHTML
			};
			this._searchSeq = 0;
			this.commands = this.buildCommands();
			if (this.gitignoreEnabled) {
				this.loadGitignoreStatus();
			}
			this.loadFiles();
			document.body.addEventListener('htmx:afterSwap', () => this.refreshFiles());
			document.body.addEventListener('indexReady', () => {
				this.indexReady = true;
				this.refreshFiles();
			});
		},

		loadFiles() {
			const filesEl = document.getElementById('markdown-files');
			if (filesEl) {
				this.allFiles = JSON.parse(filesEl.textContent).map(f => ({
					id: 'file:' + f.Path,
					type: 'file',
					name: f.Name,
					description: f.Path,
					path: f.Path
				}));
			}
			this.rebuildIndex();
			this.showDefaultResults();
		},

		showDefaultResults() {
			const files = this.allFiles.slice(0, 50).map(f => ({ ...f, matches: null }));
			this.contentResults = [];
			this.contentTruncated = false;
			this.applyResults([], files);
		},

		// applyResults assembles the display groups and the flat keyboard-nav
		// list. Content ranks above Files for prose-like queries (more than
		// two words, or no filename-ish characters), Files first otherwise.
		applyResults(commands, files) {
			this._commands = commands;
			this._files = files;

			const groups = [];
			if (commands.length > 0) {
				groups.push({ key: 'commands', label: 'Commands', items: commands });
			}
			const fileGroup = files.length > 0 ? { key: 'files', label: 'Files', items: files } : null;
			const contentLabel = this.contentTruncated ? 'Content (first 20)' : 'Content';
			const contentGroup = this.contentResults.length > 0 ? { key: 'content', label: contentLabel, items: this.contentResults } : null;

			const q = this.query.trim();
			const prose = q.split(/\s+/).length > 2 || !/[./_-]/.test(q);
			const ordered = prose ? [contentGroup, fileGroup] : [fileGroup, contentGroup];
			ordered.forEach(g => { if (g) groups.push(g); });

			this.results = groups.flatMap(g => g.items);
			this.results.forEach((item, i) => { item.resultIndex = i; });
			this.displayGroups = groups;
			this.selectedIndex = 0;
		},

		fetchContent(q) {
			if (!q || q.length < 2) {
				this.contentResults = [];
				this.contentTruncated = false;
				this.applyResults(this._commands || [], this._files || []);
				return;
			}
			const seq = ++this._searchSeq;
			fetch('/api/search?q=' + encodeURIComponent(q))
				.then(res => res.json())
				.then(data => {
					if (seq !== this._searchSeq || this.query.trim() !== q) return;
					this.contentResults = data.results.map(m => ({
						id: 'content:' + m.path,
						type: 'content',
						name: m.title,
						path: m.path,
						snippet: m.snippet,
						matchStart: m.matchStart,
						matchEnd: m.matchEnd
					}));
					this.contentTruncated = data.truncated;
					this.applyResults(this._commands || [], this._files || []);
				})
				.catch(err => console.error('Content search failed:', err));
		},

		snippetHtml(item) {
			const s = item.snippet;
			return '…' + this.escapeHtml(s.slice(0, item.matchStart)) +
				'<strong>' + this.escapeHtml(s.slice(item.matchStart, item.matchEnd)) + '</strong>' +
				this.escapeHtml(s.slice(item.matchEnd)) + '…';
		},

		rebuildIndex() {
			const allItems = [...this.commands, ...this.allFiles];
			this.fuse = new Fuse(allItems, {
				keys: ['name', 'description'],
				threshold: 0.4,
				useExtendedSearch: true,
				includeScore: true,
				includeMatches: true
			});
		},

		refreshFiles() {
			fetch('/api/files')
				.then(res => res.json())
				.then(files => {
					this.allFiles = files.map(f => ({
						id: 'file:' + f.Path,
						type: 'file',
						name: f.Name,
						description: f.Path,
						path: f.Path
					}));
					this.rebuildIndex();
					if (this.query.trim()) {
						this.filter();
					} else {
						this.showDefaultResults();
					}
				})
				.catch(err => console.error('Failed to refresh files:', err));
		},

		togglePalette() {
			if (this.open) {
				this.close();
			} else {
				this.open = true;
				this.$nextTick(() => this.$refs.searchInput.focus());
			}
		},

		close() {
			this.open = false;
			this.query = '';
			this.showDefaultResults();
		},

		debouncedFilter() {
			clearTimeout(this._filterTimeout);
			this._filterTimeout = setTimeout(() => this.filter(), 150);
		},

		filter() {
			const q = this.query.trim();

			if (!q) {
				this.showDefaultResults();
				return;
			}

			let commands, files;
			if (q.length === 1) {
				const lower = q.toLowerCase();
				commands = this.commands.filter(c => c.name.toLowerCase().startsWith(lower)).map(c => ({ ...c, matches: null }));
				files = this.allFiles.filter(f => f.name.toLowerCase().startsWith(lower)).slice(0, 50).map(f => ({ ...f, matches: null }));
			} else {
				const searchResults = this.fuse.search(q, { limit: 50 });
				commands = searchResults.filter(r => r.item.type === 'command').map(r => ({ ...r.item, matches: r.matches }));
				files = searchResults.filter(r => r.item.type === 'file').map(r => ({ ...r.item, matches: r.matches }));
			}

			this.applyResults(commands, files);
			this.fetchContent(q);
		},

		highlightMatch(item) {
			if (!item.matches) {
				return this.escapeHtml(item.name);
			}

			const nameMatch = item.matches.find(m => m.key === 'name');
			if (!nameMatch) {
				return this.escapeHtml(item.name);
			}

			const text = item.name;
			const indices = nameMatch.indices;
			let result = '';
			let lastIndex = 0;

			for (const [start, end] of indices) {
				result += this.escapeHtml(text.slice(lastIndex, start));
				result += '<strong>' + this.escapeHtml(text.slice(start, end + 1)) + '</strong>';
				lastIndex = end + 1;
			}
			result += this.escapeHtml(text.slice(lastIndex));

			return result;
		},

		escapeHtml(text) {
			return text
				.replace(/&/g, '&amp;')
				.replace(/</g, '&lt;')
				.replace(/>/g, '&gt;')
				.replace(/"/g, '&quot;');
		},

		nextResult() {
			if (this.selectedIndex < this.results.length - 1) {
				this.selectedIndex++;
				this.scrollToSelected();
			}
		},

		prevResult() {
			if (this.selectedIndex > 0) {
				this.selectedIndex--;
				this.scrollToSelected();
			}
		},

		scrollToSelected() {
			this.$nextTick(() => {
				const container = this.$refs.resultsList;
				const selected = container?.querySelector('[data-selected="true"]');
				if (selected) {
					selected.scrollIntoView({ block: 'nearest' });
				}
			});
		},

		selectResult() {
			if (this.results.length > 0) {
				this.selectIndex(this.selectedIndex);
			}
		},

		selectIndex(index) {
			if (index >= 0 && index < this.results.length) {
				const item = this.results[index];
				this.close();
				if (item.type === 'command') {
					item.action();
				} else {
					// Files and content matches both navigate to the document.
					htmx.ajax('GET', '/file' + item.path, { source: this.$root, target: '#content', swap: 'innerHTML' });
				}
			}
		}
	}
}
