import json
import logging

from persistent import Persistent
from pyramid.view import view_config

from ..content import content
from ..folder import Folder
from ..util import find_service


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


@content('Channel')
class Channel(Folder):
    pass


class ChannelPhoto(Persistent):
    def __init__(self, mimetype, url_large, url_medium, url_small, date):
        super().__init__()
        self.mimetype = mimetype
        self.url_large = url_large
        self.url_medium = url_medium
        self.url_small = url_small
        self.date = date


@content('ZotHub')
class ZotHub(Folder):
    def __init__(self, hub_hash, guid, signature,
            address, url, url_signature, host,
            callback, key):
        super().__init__()
        self.hub_hash = hub_hash
        self.guid = guid
        self.signature = signature
        self.address = address
        self.url = url
        self.url_signature = url_signature
        self.host = host
        self.callback = callback
        self.key = key


@content('ZotChannel')
class ZotChannel(Folder):
    def __init__(self, channel_hash, name, address, guid, signature, key):
        super().__init__()
        self.channel_hash = channel_hash
        self.name = name
        self.address = address
        self.guid = guid
        self.signature = signature
        self.key = key


@content('ZotXChannel')
class ZotXChannel(Folder):
    def __init__(self, channel_hash, guid, signature,
                 address, url, connections_url,
                 name, photo, flags, key):
        super().__init__()
        self.channel_hash = channel_hash
        self.guid = guid
        self.signature = signature
        self.address = address
        self.url = url
        self.connections_url = connections_url
        self.name = name
        self.photo = photo
        self.flags = flags
        self.key = key


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
