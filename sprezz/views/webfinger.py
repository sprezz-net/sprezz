import json
import logging

import furl
import trafaret as T
import venusian

from aiohttp import web
from trafaret_validator import TrafaretValidator

from sprezz.utils.accept import AcceptChooser


log = logging.getLogger(__name__)


# Make acct known as scheme
# https://github.com/gruns/furl/issues/97
if 'acct' not in furl.COLON_SEPARATED_SCHEMES:
    furl.COLON_SEPARATED_SCHEMES.append('acct')


RESOURCE_ALLOWED_SCHEMES = ['acct', 'http', 'https', 'mailto']
RESOURCE_STANDARD_PORTS = ['80', '443']


chooser = AcceptChooser()  # pylint: disable=invalid-name


class WebFingerRegistry:
    def __init__(self):
        self._aliases = []
        self._properties = []
        self._links = {}
        self._frozen = False

    def _check_frozen(self):
        if self._frozen:
            raise RuntimeError('Cannot modify frozen list.')

    @property
    def aliases(self):
        return self._aliases

    @property
    def properties(self):
        return self._properties

    @property
    def links(self):
        return self._links

    def add_alias(self, func):
        self._check_frozen()
        self._alias.add(func)

    def add_property(self, func):
        self._check_frozen()
        self._properties.add(func)

    def add_link(self, rel, func):
        self._check_frozen()
        self._links[rel] = func


class webfinger:
    def __init__(self, alias=False, prop=False, rel=None):
        self.alias = alias
        self.prop = prop
        self.rel = rel

    def __call__(self, func):
        def callback(scanner, name, ob):
            try:
                reg = scanner.registry['webfinger']
            except KeyError:
                reg = WebFingerRegistry()
                scanner.registry['webfinger'] = reg
            if self.alias:
                reg.add_alias(ob)
            if self.prop:
                reg.add_property(ob)
            if self.rel is not None:
                reg.add_link(rel=self.rel, func=ob)
        venusian.attach(func, callback)
        return func

    @classmethod
    def alias(cls):
        return cls(alias=True)

    @classmethod
    def prop(cls):
        return cls(prop=True)

    @classmethod
    def rel(cls, rel):
        return cls(rel=rel)


async def webfinger_plugins(request, account, resource, rels=None):
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

    reg = request.app['registry'].get('webfinger')
    for plugin in reg.aliases:
        part = await plugin(request=request,
                            account=account,
                            resource=resource,
                            rels=rels)
        if part:
            if 'aliases' not in result:
                result['aliases'] = []
            result['aliases'].extend(part)

    for plugin in reg.properties:
        part = await plugin(request=request,
                            account=account,
                            resource=resource,
                            rels=rels)
        if part:
            if 'properties' not in result:
                result['properties'] = {}
            result['properties'].update(part)

    if rels is None:
        # Load all link plugins when no relation is given.
        plugin_keys = reg.links.keys()
    else:
        # Otherwise restrict to the specified relations.
        plugin_keys = rels
    for key in plugin_keys:
        plugin = reg.links[key]
        part = await plugin(request=request,
                            account=account,
                            resource=resource,
                            rels=rels)
        if part:
            if 'links' not in result:
                result['links'] = []
            result['links'].extend(part)

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

    def asdict(self):
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
        # TODO Check if account exists.
        result = await webfinger_plugins(account=account, request=request,
                                         resource=resource, rels=rels)
    else:
        # TODO Allow WebFinger on other type of objects like articles
        # and respond with copyright information, for example.
        raise web.HTTPNotFound()

    return web.json_response(result, content_type='application/jrd+json')


def setup_routes(app: web.Application) -> None:
    app.add_routes([web.route('*', '/.well-known/webfinger', chooser.route)])
