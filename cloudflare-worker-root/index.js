export default {
  async fetch(request, env) {
    const reqURL = new URL(request.url)
    const scalekitBase = String(env.SCALEKIT_BASE_URL || 'https://hookweb.scalekit.com').replace(/\/+$/, '')
    const appRedirectBase = String(env.APP_REDIRECT_BASE || 'https://app.agenthook.store').replace(/\/+$/, '')

    if (reqURL.pathname === '/auth/scalekit/login') {
      const loginURL = new URL(`${scalekitBase}/a/auth/login`)
      loginURL.searchParams.set('redirect_uri', `${appRedirectBase}/auth/scalekit/callback`)
      return Response.redirect(loginURL.toString(), 302)
    }

    if (reqURL.pathname === '/auth/scalekit/signup') {
      const signupURL = new URL(`${scalekitBase}/a/auth/signup`)
      signupURL.searchParams.set('redirect_uri', `${appRedirectBase}/auth/scalekit/callback`)
      return Response.redirect(signupURL.toString(), 302)
    }

    if (reqURL.pathname === '/auth/scalekit/callback') {
      const appURL = new URL(`${appRedirectBase}/`)
      const code = reqURL.searchParams.get('code')
      if (code) appURL.searchParams.set('code', code)
      return Response.redirect(appURL.toString(), 302)
    }

    const response = await env.ASSETS.fetch(request)
    const out = new Response(response.body, response)
    out.headers.set('x-agenthook-site-source', 'cloudflare-worker-static-website')
    out.headers.set('cache-control', 'public, max-age=0, must-revalidate')
    out.headers.set('x-content-type-options', 'nosniff')
    out.headers.set('referrer-policy', 'strict-origin-when-cross-origin')
    return out
  },
}
