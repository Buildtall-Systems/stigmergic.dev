module.exports = {
  content: [
    "./web/templates/**/*.templ",
    "./web/templates/**/*.go",
    "./internal/embed/web/static/js/**/*.js",
    "./vendor/github.com/buildtall-systems/buildtall/btk/views/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        selection: 'var(--selection-color)',
        // The btk login views name these roles; map them onto the theme.
        primary: 'var(--cyan-color)',
        secondary: 'var(--blue-color)',
        bg: 'var(--bg-color)',
        'bg-subtle': 'var(--bg-alt-color)',
        fg: 'var(--fg-color)',
        'fg-subtle': 'var(--comment-color)',
        'fg-muted': 'var(--comment-color)',
        danger: 'var(--red-color)',
      },
      typography: {
        DEFAULT: {
          css: {
            maxWidth: 'none',
            color: 'inherit',
            a: {
              color: 'var(--link-color)',
              textDecoration: 'underline',
              '&:hover': {
                color: 'var(--link-hover-color)',
              },
            },
            code: {
              color: 'var(--code-fg-color)',
              backgroundColor: 'var(--code-bg-color)',
              padding: '0.25rem 0.375rem',
              borderRadius: '0.25rem',
              fontWeight: '600',
            },
            'code::before': {
              content: '""',
            },
            'code::after': {
              content: '""',
            },
            pre: {
              backgroundColor: 'var(--code-bg-color)',
              color: 'var(--fg-color)',
            },
            'pre code': {
              backgroundColor: 'transparent',
              color: 'inherit',
              padding: 0,
            },
          },
        },
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
}
