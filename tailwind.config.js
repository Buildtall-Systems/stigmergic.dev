module.exports = {
  content: [
    "./web/templates/**/*.templ",
    "./web/templates/**/*.go",
    "./internal/embed/web/static/js/**/*.js",
  ],
  theme: {
    extend: {
      colors: {
        selection: 'var(--selection-color)',
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
