import json
import logging

import attr
import furl
import trafaret as T

from aiohttp import web
from trafaret_validator import TrafaretValidator

from sprezz.utils.accept import AcceptChooser


log = logging.getLogger(__name__)


# Make acct known as scheme
# https://github.com/gruns/furl/issues/97
furl.COLON_SEPARATED_SCHEMES.append('acct')


chooser = AcceptChooser()  # pylint: disable=invalid-name


async def openid_issuer(account, request, resource, rel=None):
    return {
        'links': [{
            'rel': rel,
            'href': 'https://{netloc}'.format(
                netloc=request.app['config']['netloc'])}]}


WEBFINGER_RELS = {
    'http://openid.net/specs/connect/1.0/issuer': openid_issuer,
    }


RESOURCE_ALLOWED_SCHEMES = ['acct', 'http', 'https', 'mailto']
RESOURCE_STANDARD_PORTS = ['80', '443']


async def webfinger_account(account, request, resource, rels=None):
    result = {}
    result['subject'] = 'acct:{account}@{netloc}'.format(
        account=account,
        netloc=request.app['config']['netloc'])
    result['aliases'] = [
        'https://{netloc}/{account}'.format(
            netloc=request.app['config']['netloc'],
            account=account),
        'https://{netloc}/~{account}'.format(
            netloc=request.app['config']['netloc'],
            account=account),
        'https://{netloc}/@{account}'.format(
            netloc=request.app['config']['netloc'],
            account=account)
        ]
    if rels is None:
        rels = WEBFINGER_RELS
    for rel in rels:
        method = WEBFINGER_RELS[rel]
        part = await method(account, request, resource, rel)
        for section in ['properties', ]:
            if section in part:
                if section not in result:
                    result[section] = {}
                result[section].update(part[section])
        for section in ['aliases', 'links']:
            if section in part:
                if section not in result:
                    result[section] = []
                result[section].extend(part[section])
    return result


class MDKey(T.Key):
    def get_data(self, data, default):
        return data.getall(self.name, default)


class WebfingerValidator(TrafaretValidator):
    query = T.Dict({T.Key('resource'): T.String,
                    MDKey('rel', optional=True): T.List(T.String),
                   },
                   ignore_extra='*')


class Resource:
    def __init__(self, resource):
        self._url = furl.furl(resource)
        self.normalize()

    def normalize(self):
        if self.scheme is None:
            # https://openid.net/specs/openid-connect-discovery-1_0.html#NormalizationSteps
            # Default to https scheme when not specified.
            self._url = furl.furl('https://{}'.format(self._url.url))
            # Default to acct when only username and host are defined.
            if self.username and self.host and (self.port is None) and (
                    self.path == '') and self.query == '' and (
                        self.fragment == ''):
                self._url.set(scheme='acct')
        self._url.path.normalize()
        self._url.fragment.path.normalize()

    @property
    def scheme(self):
        return self._url.scheme

    @property
    def username(self):
        return self._url.username

    @property
    def host(self):
        return self._url.host

    @property
    def port(self):
        return self._url._port

    @property
    def path(self):
        return str(self._url.path)

    @property
    def path_segments(self):
        return self._url.path.segments

    @property
    def query(self):
        return str(self._url.query)

    @property
    def fragment(self):
        return str(self._url.fragment)

    @property
    def fragment_segments(self):
        return self._url.fragment.segments

    @property
    def url(self):
        return self._url.url

    def __str__(self):
        return self.url

    def as_dict(self):
        keys = ['scheme', 'username', 'host', 'port',
                'path', 'query', 'fragment']
        data = {}
        for key in keys:
            data[key] = getattr(self, key)
        return data


@chooser.accept('GET', 'application/json')
@chooser.accept('GET', 'application/jrd+json')
async def webfinger_get_jrd(request):

    def raise_bad_request(errors):
        data = {'errors': errors}
        body = json.dumps(data)
        raise web.HTTPBadRequest(body=body, content_type='application/json')

    def raise_temp_redirect(request, resource, rels=None):
        location = furl.furl().set(scheme='https',
                                   host=resource.host,
                                   path='/.well-known/webfinger')
        if resource.port and resource.port not in RESOURCE_STANDARD_PORTS:
            location.set(port=resource.port)
        location.add(query_params={'resource': resource.url})
        if rels is not None:
            for rel in rels:
                location.add(query_params={'rel': rel})
        raise web.HTTPTemporaryRedirect(location=location.url)

    if not request.secure:
        raise web.HTTPInternalServerError(reason='WebFinger requires HTTPS')
    validator = WebfingerValidator(query=request.query)
    if not validator.validate():
        raise_bad_request(validator.errors['query'])
    data = validator.data['query']
    try:
        resource = Resource(data.get('resource'))
    except (ValueError, AttributeError) as err:
        raise_bad_request(str(err))

    rels = data.get('rel')
    if resource.host != request.app['config']['host']:
        raise_temp_redirect(request, resource, rels)
    if resource.port and resource.port != request.app['config']['port']:
        raise_temp_redirect(request, resource, rels)
    if resource.scheme not in RESOURCE_ALLOWED_SCHEMES:
        raise web.HTTPNotFound()

    account = None
    if resource.scheme in ['acct', 'mailto']:
        # acct:account@host
        # mailto:account@host
        account = resource.username
    elif resource.scheme in ['http', 'https']:
        if resource.username:
            # https://account@host
            account = resource.username
        elif len(resource.path_segments) == 1:
            # https://host/account
            account = resource.path_segments[0]
            if account.startswith('~') or account.startswith('@'):
                # https://host/~account
                # https://host/@account
                account = account[1:]
        elif len(resource.path_segments) == 2:
            if resource.path_segments[0] in ['users', 'channel']:
                # https://host/users/account
                # https://host/channel/account
                account = resource.path_segments[1]
    if account is not None:
        result = await webfinger_account(account, request, resource, rels)
    else:
        # TODO Allow WebFinger on other type of objects like articles
        # and respond with copyright information, for example.
        raise web.HTTPNotFound()

    return web.json_response(result, content_type='application/jrd+json')


def setup_routes(app: web.Application) -> None:
    app.add_routes([web.route('*', '/.well-known/webfinger', chooser.route)])
