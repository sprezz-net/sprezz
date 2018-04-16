import attr
import json
import logging

import furl
import trafaret as T

from urllib.parse import urlsplit

from aiohttp import web
from trafaret_validator import TrafaretValidator

from sprezz.utils.accept import AcceptChooser


log = logging.getLogger(__name__)


# Make acct known as scheme
# https://github.com/gruns/furl/issues/97
furl.COLON_SEPARATED_SCHEMES.append('acct')


chooser = AcceptChooser()  # pylint: disable=invalid-name


async def link_openid_issuer(resource, rel):
    return


WEBFINGER_RELS = {
    'http://openid.net/specs/connect/1.0/issuer': link_openid_issuer,
    }


RESOURCE_ALLOWED_SCHEMES = ['acct', 'http', 'https', 'mailto']
RESOURCE_STANDARD_PORTS = ['80', '443']


class MDKey(T.Key):
    def get_data(self, data, default):
        return data.getall(self.name, default)


class WebfingerValidator(TrafaretValidator):
    query = T.Dict({
        T.Key('resource'): T.String,
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
            if self.username and self.host and self.port is None and (
                    self.path == '' and self.query == '' and (
                    self.fragment == '')):
                self._url.set(scheme='acct')
        if self.path != '':
            self._url.path.normalize()

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
    def query(self):
        return str(self._url.query)

    @property
    def fragment(self):
        return str(self._url.fragment)

    @property
    def url(self):
        return self._url.url

    def __str__(self):
        return self.url

    def asdict(self):
        return {'scheme': self.scheme,
                'username': self.username,
                'host': self.host,
                'port': self.port,
                'path': self.path,
                'query': self.query,
                'fragment': self.fragment}


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
    except (ValueError, AttributeError) as e:
        raise_bad_request(str(e))

    rel = data.get('rel')
    if resource.host != request.app['config']['host']:
        raise_temp_redirect(request, resource, rel)
    if resource.port and resource.port != request.app['config']['port']:
        raise_temp_redirect(request, resource, rel)
    if resource.scheme not in RESOURCE_ALLOWED_SCHEMES:
        raise web.HTTPNotFound()

    # TODO Return an actual WebFinger response
    log.debug(resource)
    return web.json_response(resource.asdict())


def setup_routes(app: web.Application) -> None:
    app.add_routes([web.route('*', '/.well-known/webfinger', chooser.route)])
