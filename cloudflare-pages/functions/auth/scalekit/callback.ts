export const onRequestGet: PagesFunction = async ({ request }) => {
  const reqUrl = new URL(request.url)
  const code = reqUrl.searchParams.get('code')
  const target = new URL('https://app.agenthook.store/')
  if (code) target.searchParams.set('code', code)
  return Response.redirect(target.toString(), 302)
}
