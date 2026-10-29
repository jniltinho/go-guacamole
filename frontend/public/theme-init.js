// Applies the saved theme before first paint (external file: the CSP forbids inline scripts).
;(function () {
  try {
    var t = localStorage.getItem('gg-theme')
    if (!t)
      t = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    document.documentElement.setAttribute('data-theme', t)
  } catch {
    /* storage unavailable */
  }
})()
