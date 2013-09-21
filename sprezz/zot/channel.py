import logging

from persistent import Persistent
from pyramid.traversal import find_root, resource_path
from pyramid.view import view_config
from zope.interface import implementer

from ..content import content
from ..folder import Folder
from ..interfaces import IZotHub, IZotChannel, IZotXChannel
from ..util.base64 import base64_url_encode
from ..util.folder import find_service


log = logging.getLogger(__name__)


@content('ZotHubs')
class ZotHubs(Folder):
    pass


@content('ZotChannels')
class ZotChannels(Folder):
    pass


@content('ZotXChannels')
class ZotXChannels(Folder):
    pass


class ChannelPhoto(Persistent):
    def __init__(self, mimetype, url_large, url_medium, url_small, date):
        super().__init__()
        self.mimetype = mimetype
        self.url_large = url_large
        self.url_medium = url_medium
        self.url_small = url_small
        self.date = date


@content('ZotLocalHub')
@implementer(IZotHub)
class ZotLocalHub(Folder):
    def __init__(self, channel_hash, guid, signature, key):
        super().__init__()
        self.channel_hash = channel_hash
        self.guid = guid
        self.signature = signature
        self.key = key

    @property
    def host(self):
        try:
            return self._v_host
        except AttributeError:
            root = find_root(self)
            host = root.netloc
            self._v_host = host
            return host

    @property
    def address(self):
        try:
            return self._v_address
        except AttributeError:
            xchannel_service = find_service(self, 'zot', 'xchannel')
            address = xchannel_service[self.channel_hash].address
            self._v_address = address
            return address

    @property
    def url(self):
        try:
            return self._v_url
        except AttributeError:
            root = find_root(self)
            url = root.app_url
            self._v_url = url
            return url

    @property
    def url_signature(self):
        try:
            return self._v_url_signature
        except AttributeError:
            channel_service = find_service(self, 'zot', 'channel')
            xchannel_service = find_service(self, 'zot', 'xchannel')
            nickname = xchannel_service[self.channel_hash].nickname
            url_signature = channel_service[nickname].sign_url(self.url)
            self._v_url_signature = url_signature
            return url_signature

    @property
    def callback(self):
        try:
            return self._v_callback
        except AttributeError:
            root = find_root(self)
            endpoint_service = find_service(self, 'zot', 'post')
            url = '{0}{1}'.format(root.app_url,
                                  resource_path(endpoint_service))
            self._v_callback = url
            return url

    def update(self, data):
        pass


@content('ZotRemoteHub')
@implementer(IZotHub)
class ZotRemoteHub(Folder):
    def __init__(self, channel_hash, guid, signature, key,
                 host, address, url, url_signature, callback):
        super().__init__()
        self.channel_hash = channel_hash
        self.guid = guid
        self.signature = signature
        self.key = key
        self.host = host
        self.address = address
        self.url = url
        self.url_signature = url_signature
        self.callback = callback

    def update(self, data):
        keys = ['host', 'address',
                'url', 'url_signature', 'callback']
        for x in keys:
            if (x in data) and (getattr(self, x) != data[x]):
                log.debug('hub.update() attr={0}, '
                          'old={1}, new={2}'.format(x,
                                                    getattr(self, x),
                                                    data[x]))
                setattr(self, x, data[x])


@content('ZotLocalChannel')
@implementer(IZotChannel)
class ZotLocalChannel(Folder):
    def __init__(self, nickname, name, channel_hash, guid, signature, key):
        super().__init__()
        self.nickname = nickname
        self.name = name
        self.channel_hash = channel_hash
        self.guid = guid
        self.signature = signature
        self.key = key

    def __json__(self, request):
        return {'id': self.channel_hash,
                'nickname': self.nickname,
                'name': self.name}

    def sign_url(self, url):
        return base64_url_encode(self.key.sign_message(url))


@content('ZotLocalXChannel')
@implementer(IZotXChannel)
class ZotLocalXChannel(Folder):
    def __init__(self, nickname, name, channel_hash, guid, signature, key,
                 photo, flags):
        super().__init__()
        self.nickname = nickname
        self.name = name
        self.channel_hash = channel_hash
        self.guid = guid
        self.signature = signature
        self.key = key
        self.photo = photo
        self.flags = flags

    @property
    def address(self):
        try:
            return self._v_address
        except AttributeError:
            root = find_root(self)
            address = '@'.join([self.nickname, root.netloc])
            self._v_address = address
            return address

    @property
    def url(self):
        try:
            return self._v_url
        except AttributeError:
            root = find_root(self)
            url = '/'.join([root.app_url, self.nickname])
            self._v_url = url
            return url

    @property
    def connections_url(self):
        try:
            return self._v_connections_url
        except AttributeError:
            root = find_root(self)
            poco_service = find_service(self, 'zot', 'poco')
            url = '{0}{1}'.format(root.app_url, resource_path(poco_service,
                                                              self.nickname))
            self._v_connections_url = url
            return url

    def update(self, data):
        pass


@content('ZotRemoteXChannel')
@implementer(IZotXChannel)
class ZotRemoteXChannel(Folder):
    def __init__(self, nickname, name, channel_hash, guid, signature, key,
                 address, url, connections_url, photo, flags):
        super().__init__()
        self.nickname = nickname
        self.name = name
        self.channel_hash = channel_hash
        self.guid = guid
        self.signature = signature
        self.key = key
        self.address = address
        self.url = url
        self.connections_url = connections_url
        self.photo = photo
        self.flags = flags

    def update(self, data):
        keys = ['nickname', 'name', 'address',
                'url', 'connections_url', 'flags']
        for x in keys:
            if (x in data) and (getattr(self, x) != data[x]):
                log.debug('xchannel.update() attr={0}, '
                          'old={1}, new={2}'.format(x,
                                                    getattr(self, x),
                                                    data[x]))
                setattr(self, x, data[x])


class ChannelView(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotChannels,
                 request_method='POST',
                 renderer='json')
    def create_channel(self):
        # TODO validate name and nickname (colander)
        nickname = self.request.params['nickname'].strip().lower()
        name = self.request.params['name'].strip()
        zot_service = find_service(self.context, 'zot')
        channel = zot_service.add_channel(nickname, name)
        return channel.__json__(self.request)

    @view_config(context=ZotChannels,
                 request_method='GET',
                 renderer='json')
    def list_channels(self):
        return {'channels': [c.__json__(self.request) for c in
                             self.context.values()]}

    @view_config(context=ZotLocalChannel,
                 request_method='POST',
                 name='connection',
                 renderer='json')
    def add_connection(self):
        result = {'success': False}
        url = self.request.params['url'].strip()
        zot_service = find_service(self.context, 'zot')
        #try:
        info = zot_service.zot_finger(url, self.context.channel_hash)
        #except Exception as e:
            # TODO Check for the various exception classes like HTTP
            # exeception, etc.
            #result['message'] = str(e)
            #return result

        zot_service.import_xchannel(info)

        result['success'] = True
        result['info'] = info
        return result
