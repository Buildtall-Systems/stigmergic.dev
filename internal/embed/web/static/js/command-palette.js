function commandPalette() {
	return {
		open: false,
		query: '',
		allFiles: [],
		respectGitignore: true,
		commands: [],
		results: [],
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

		get groupedResults() {
			return this._groupedResults || { commands: [], files: [] };
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
				return: document.getElementById('icon-return').innerHTML
			};
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
			this.results = this.allFiles.slice(0, 50).map((f, i) => ({ ...f, matches: null, resultIndex: i }));
			this._groupedResults = {
				commands: [],
				files: this.results
			};
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
						this.results = this.allFiles.slice(0, 50).map((f, i) => ({ ...f, matches: null, resultIndex: i }));
						this._groupedResults = { commands: [], files: this.results };
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
			this.selectedIndex = 0;
			this.results = this.allFiles.slice(0, 50).map((f, i) => ({ ...f, matches: null, resultIndex: i }));
			this._groupedResults = { commands: [], files: this.results };
		},

		debouncedFilter() {
			clearTimeout(this._filterTimeout);
			this._filterTimeout = setTimeout(() => this.filter(), 150);
		},

		filter() {
			const q = this.query.trim();

			if (!q) {
				this.results = this.allFiles.slice(0, 50).map((f, i) => ({ ...f, matches: null, resultIndex: i }));
				this._groupedResults = { commands: [], files: this.results };
				this.selectedIndex = 0;
				return;
			}

			if (q.length === 1) {
				const lower = q.toLowerCase();
				const commands = this.commands.filter(c => c.name.toLowerCase().startsWith(lower));
				const files = this.allFiles.filter(f => f.name.toLowerCase().startsWith(lower));
				this.results = [...commands, ...files].slice(0, 50).map((item, i) => ({ ...item, matches: null, resultIndex: i }));
				this._groupedResults = {
					commands: this.results.filter(r => r.type === 'command'),
					files: this.results.filter(r => r.type === 'file')
				};
				this.selectedIndex = 0;
				return;
			}

			const searchResults = this.fuse.search(q, { limit: 50 });
			const commands = searchResults.filter(r => r.item.type === 'command').map(r => ({ ...r.item, matches: r.matches }));
			const files = searchResults.filter(r => r.item.type === 'file').map(r => ({ ...r.item, matches: r.matches }));
			this.results = [...commands, ...files].map((item, i) => ({ ...item, resultIndex: i }));
			this._groupedResults = {
				commands: this.results.filter(r => r.type === 'command'),
				files: this.results.filter(r => r.type === 'file')
			};
			this.selectedIndex = 0;
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
					htmx.ajax('GET', '/file' + item.path, { source: this.$root, target: '#content', swap: 'innerHTML' });
				}
			}
		}
	}
}
