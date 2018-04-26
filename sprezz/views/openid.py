from sprezz.views.webfinger import webfinger


@webfinger.rel('http://openid.net/specs/connect/1.0/issuer')
async def openid_issuer(request, account, resource, rels=None):
    return [{
        'rel': 'http://openid.net/specs/connect/1.0/issuer',
        'href': 'https://{netloc}'.format(
            netloc=request.app['config']['netloc'])}]
