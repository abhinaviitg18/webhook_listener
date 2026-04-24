export const onRequestGet: PagesFunction = async () => {
  const loginUrl = new URL('https://hookweb.scalekit.com/a/auth/login')
  loginUrl.searchParams.set('redirect_uri', 'https://app.agenthook.store/auth/scalekit/callback')
  return Response.redirect(loginUrl.toString(), 302)
}
