import json
import logging

from persistent import Persistent
from pyramid.traversal import find_root, resource_path
from pyramid.view import view_config
from zope.interface import implementer

from ..content import content
from ..folder import Folder
from ..interfaces import IZotHub, IZotChannel, IZotXChannel
from ..util import find_service, base64_url_encode


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
    def address(self):
        try:
            return self._v_address
        except AttributeError:
            xchannel = find_service(self, 'zot', 'xchannel')
            address = xchannel[self.channel_hash].address
            self._v_address = address
            return address

    @property
    def url(self):
        try:
            return self._v_url
        except AttributeError:
            root = find_root(self)
            url = root.hostname
            if root.port != 80 and root.port != 443:
                url = '{0}:{1:d}'.format(url, root.port)
            self._v_url = url
            return url

    @property
    def url_signature(self):
        try:
            return self._v_url_signature
        except AttributeError:
            channel = find_service(self, 'zot', 'channel')
            xchannel = find_service(self, 'zot', 'xchannel')
            nickname = xchannel[self.channel_hash].nickname
            url_signature = channel[nickname].sign_hub_url(self.url)
            self._v_url_signature = url_signature
            return url_signature

    @property
    def callback(self):
        try:
            return self._v_callback
        except AttributeError:
            root = find_root(self)
            endpoint = find_service(self, 'zot', 'post')
            url = '{0}{1}'.format(root.app_url, resource_path(endpoint))
            self._v_callback = url
            return url


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

    def sign_hub_url(self, url):
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
            address = '@'.join([self.nickname, root.hostname])
            if root.port != 80 and root.port != 443:
                address = '{0}:{1:d}'.format(address, root.port)
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
            poco = find_service(self, 'zot', 'poco')
            url = '{0}{1}'.format(root.app_url, resource_path(poco,
                                                              self.nickname))
            self._v_connections_url = url
            return url


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
        zot_service.add_channel(nickname, name)
        return { 'nickname': nickname, 'name': name }

    @view_config(context=ZotChannels,
                 request_method='GET',
                 renderer='json')
    def list_channels(self):
        l = list(self.context.keys())
        return json.dumps(l)
