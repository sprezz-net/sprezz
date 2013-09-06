import logging
import random
import sys
import whirlpool

from pyramid.threadlocal import get_current_registry
from pyramid.traversal import find_root, resource_path

from ..content import service
from ..crypto import PersistentRSAKey
from ..folder import Folder
from ..util import base64_url_encode


log = logging.getLogger(__name__)


@service('Zot', service_name='zot', after_create='after_create')
class Zot(Folder):
    def after_create(self, inst, registry):
        log.info('Generating RSA keys for this site')
        self._private_site_key = PersistentRSAKey()
        self._private_site_key.generate_keypair()
        self._public_site_key = self._private_site_key.get_public_key()
        log.info('Registering Zot services')
        hub = registry.content.create('ZotHubs')
        channel = registry.content.create('ZotChannels')
        xchannel = registry.content.create('ZotXChannels')
        endpoint = registry.content.create('ZotEndpoint')
        poco = registry.content.create('ZotPoco')
        self.add('hub', hub, registry=registry)
        self.add('channel', channel, registry=registry)
        self.add('xchannel', xchannel, registry=registry)
        self.add('post', endpoint, registry=registry)
        self.add('poco', poco, registry=registry)

    @property
    def public_site_key(self):
        return self._public_site_key

    def add_channel(self, nickname, name, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()

        prv_key = PersistentRSAKey()
        prv_key.generate_keypair()
        pub_key = prv_key.get_public_key()

        guid = self._create_channel_guid(nickname)
        signature = self._create_channel_signature(guid, prv_key)
        channel_hash = self._create_channel_hash(guid, signature)

        xchannel = registry.content.create('ZotLocalXChannel',
                nickname=nickname,
                name=name, # TODO move to graph?
                channel_hash=channel_hash,
                guid=guid,
                signature=signature,
                key=pub_key,
                photo=None,
                flags=None,
                *arg, **kw)
        self['xchannel'].add(channel_hash, xchannel)
        
        channel = registry.content.create('ZotLocalChannel',
                nickname=nickname,
                name=name, # TODO move to graph?
                channel_hash=channel_hash,
                guid=guid, # TODO redundant with xchannel?
                signature=signature, # TODO redundant with xchannel?
                key=prv_key,
                # TODO add various channel flags
                *arg, **kw)
        self['channel'].add(nickname, channel)

        hub = registry.content.create('ZotLocalHub',
                channel_hash=channel_hash,
                guid=guid,
                signature=signature,
                key=self.public_site_key,
                *arg, **kw)
        self['hub'].add(channel_hash, hub)

        log.debug('guid = %s' % guid)
        log.debug('sig = %s' % signature)
        log.debug('sig_hash = %s' % channel_hash)
        log.debug('address = %s' % xchannel.address)
        log.debug('url = %s' % xchannel.url)
        log.debug('connections url = %s' % xchannel.connections_url)
        log.debug('hub url = %s' % hub.url)
        log.debug('hub url sig = %s' % hub.url_signature)
        log.debug('callback = %s' % hub.callback)

    def _create_channel_guid(self, nickname, root=None):
        if root is None:
            root = find_root(self)
        uid = '{0}/{1}.{2}'.format(root.app_url, nickname,
                                   random.randint(0, sys.maxsize)
                                   ).encode('utf-8')
        wp = whirlpool.new(uid)
        return base64_url_encode(wp.digest())

    def _create_channel_signature(self, guid, key):
        return base64_url_encode(key.sign_message(guid))

    def _create_channel_hash(self, guid, signature):
        """Create base64 encoded channel hash.

        Arguments 'guid' and 'signature' must be base64 encoded.
        """
        message = '{0}{1}'.format(guid, signature).encode('ascii')
        wp = whirlpool.new(message)
        return base64_url_encode(wp.digest())
