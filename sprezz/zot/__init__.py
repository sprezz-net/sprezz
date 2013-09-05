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

    def get_public_site_key(self):
        return self._public_site_key

    public_site_key = property(get_public_site_key)

    def add_channel(self, nickname, name, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()

        root = find_root(self)

        prv_key = PersistentRSAKey()
        prv_key.generate_keypair()
        pub_key = prv_key.get_public_key()

        guid = self._create_channel_guid(nickname, root)
        sig = self._create_channel_signature(guid, prv_key)
        sig_hash = self._create_channel_hash(guid, sig)
        address = self._create_channel_address(nickname, root)
        url = self._create_channel_url(nickname, root)
        conn_url = self._create_channel_connections_url(nickname, root)
        host = self._create_hub_hostname(root)
        hub_url = root.app_url
        hub_url_sig = self._create_hub_url_signature(hub_url, prv_key)
        callback = self._create_hub_callback(root)

        log.debug('guid = %s' % guid)
        log.debug('sig = %s' % sig)
        log.debug('sig_hash = %s' % sig_hash)
        log.debug('host = %s' % host)
        log.debug('address = %s' % address)
        log.debug('url = %s' % url)
        log.debug('connections url = %s' % conn_url)
        log.debug('hub url = %s' % hub_url)
        log.debug('hub url sig = %s' % hub_url_sig)
        log.debug('callback = %s' % callback)

        xchannel = registry.content.create('ZotXChannel',
                channel_hash=sig_hash,
                guid=guid,
                signature=sig,
                address=address, # TODO move to graph?
                url=url,
                connections_url=conn_url,
                name=name, # TODO move to graph?
                photo=None,
                flags=None,
                key=pub_key,
                *arg, **kw)
        self['xchannel'].add(sig_hash, xchannel)
        
        channel = registry.content.create('ZotChannel',
                channel_hash=sig_hash,
                name=name, # TODO move to graph?
                address=nickname,
                guid=guid, # TODO redundant with xchannel?
                signature=sig, # TODO redundant with xchannel?
                key=prv_key,
                # TODO add various channel flags
                *arg, **kw)
        self['channel'].add(nickname, channel)

        hub = registry.content.create('ZotHub',
                hub_hash=sig_hash,
                guid=guid,
                signature=sig,
                address=address,
                url=hub_url,
                url_signature=hub_url_sig,
                host=host,
                callback=callback,
                key=self.public_site_key,
                *arg, **kw)
        self['hub'].add(sig_hash, hub)

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

    def _create_channel_hash(self, guid, sig):
        """Create base64 encoded channel hash.

        Arguments 'guid' and 'sig' must be base64 encoded.
        """
        message = '{0}{1}'.format(guid, sig).encode('ascii')
        wp = whirlpool.new(message)
        return base64_url_encode(wp.digest())

    def _create_channel_address(self, nickname, root=None):
        if root is None:
            root = find_root(self)
        address = '@'.join([nickname, root.hostname])
        if root.port != 80 and root.port != 443:
            address = '{0}:{1:d}'.format(address, root.port)
        return address

    def _create_channel_url(self, nickname, root=None):
        if root is None:
            root = find_root(self)
        return '/'.join([root.app_url, nickname])

    def _create_channel_connections_url(self, nickname, root=None):
        if root is None:
            root = find_root(self)
        return '{0}{1}'.format(root.app_url, resource_path(self['poco'],
                                                           nickname))

    def _create_hub_hostname(self, root=None):
        if root is None:
            root = find_root(self)
        return root.hostname

    def _create_hub_url_signature(self, url, key):
        return base64_url_encode(key.sign_message(url))

    def _create_hub_callback(self, root=None):
        if root is None:
            root = find_root(self)
        return '{0}{1}'.format(root.app_url, resource_path(self['post']))
