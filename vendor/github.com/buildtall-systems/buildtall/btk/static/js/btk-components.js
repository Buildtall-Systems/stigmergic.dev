document.addEventListener('alpine:init', function() {
    Alpine.data('btkDropdown', function() {
        return {
            open: false,
            toggle: function() { this.open = !this.open; },
            close: function() { this.open = false; }
        };
    });
});
