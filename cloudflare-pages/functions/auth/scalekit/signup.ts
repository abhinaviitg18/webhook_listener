export const onRequestGet: PagesFunction = async () => {
  const signupUrl = new URL('https://hookweb.scalekit.com/a/auth/signup')
  signupUrl.searchParams.set('redirect_uri', 'https://app.agenthook.store/auth/scalekit/callback')
  return Response.redirect(signupUrl.toString(), 302)
}
