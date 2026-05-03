export default {
  async fetch(request, env) {
    const reqURL = new URL(request.url)
    const hostname = reqURL.hostname
    const scalekitBase = String(env.SCALEKIT_BASE_URL || 'https://hookweb.scalekit.com').replace(/\/+$/, '')
    const appRedirectBase = String(env.APP_REDIRECT_BASE || 'https://app.agenthook.store').replace(/\/+$/, '')
    const appOriginURL = String(env.APP_ORIGIN_URL || '').replace(/\/+$/, '')
    const appOriginSecret = String(env.APP_ORIGIN_SHARED_SECRET || '')

    const isApiRequest = reqURL.pathname.startsWith('/v1/') || 
                         reqURL.pathname.startsWith('/api/') || 
                         reqURL.pathname.startsWith('/auth/')

    if (hostname === 'app.agenthook.store' || isApiRequest) {
      if (!appOriginURL) {
        const passthroughResponse = await fetch(request)
        const out = new Response(passthroughResponse.body, passthroughResponse)
        out.headers.set('x-agenthook-site-source', 'cloudflare-worker-origin-fallback')
        out.headers.set('cache-control', 'no-store')
        return out
      }

      const upstreamURL = new URL(reqURL.pathname + reqURL.search, `${appOriginURL}/`)
      const headers = new Headers(request.headers)
      headers.set('x-forwarded-host', hostname)
      headers.set('x-forwarded-proto', reqURL.protocol.replace(':', ''))
      headers.set('x-agenthook-origin-secret', appOriginSecret)

      const upstreamRequest = new Request(upstreamURL.toString(), {
        method: request.method,
        headers,
        body: request.body,
        redirect: 'manual',
        cf: {
          cacheTtl: 0,
          cacheEverything: false,
        },
      })

      const upstreamResponse = await fetch(upstreamRequest)
      const out = new Response(upstreamResponse.body, upstreamResponse)
      out.headers.set('x-agenthook-site-source', 'cloudflare-worker-lambda-proxy')
      out.headers.set('cache-control', 'no-store')
      return out
    }

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


    let response = await env.ASSETS.fetch(request)
    
    // SPA Fallback: If 404 and not a file (no extension), serve index.html
    if (response.status === 404 && request.method === 'GET' && !reqURL.pathname.includes('.')) {
      const indexReq = new Request(new URL('/index.html', request.url), request)
      response = await env.ASSETS.fetch(indexReq)
    }

    const out = new Response(response.body, response)
    out.headers.set('x-agenthook-site-source', 'cloudflare-worker-static-website')
    out.headers.set('cache-control', 'public, max-age=0, must-revalidate')
    out.headers.set('x-content-type-options', 'nosniff')
    out.headers.set('referrer-policy', 'strict-origin-when-cross-origin')
    return out
  },
}
