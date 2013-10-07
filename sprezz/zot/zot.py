import json
import logging
import random
import sys
import whirlpool

from pprint import pformat
from pyramid.threadlocal import get_current_registry
from pyramid.traversal import find_root, resource_path
from urllib.parse import urlparse, urlunparse
from zope.component import ComponentLookupError

from ..content import service
from ..folder import Folder
from ..interfaces import IZotChannel, IDeliverMessage
from ..util import network
from ..util.base64 import base64_url_encode, base64_url_decode
from ..util.crypto import PersistentRSAKey


log = logging.getLogger(__name__)


@service('Zot', service_name='zot', after_create='after_create')
class Zot(Folder):
    def after_create(self, inst, registry):
        log.info('Generating RSA keys for this site')
        self._private_site_key = PersistentRSAKey()
        self._private_site_key.generate_keypair()
        self._public_site_key = self._private_site_key.get_public_key()

        log.info('Registering Zot services')
        site_service = registry.content.create('ZotSites')
        hub_service = registry.content.create('ZotHubs')
        channel_service = registry.content.create('ZotChannels')
        xchannel_service = registry.content.create('ZotXChannels')
        message_service = registry.content.create('Messages')
        endpoint_service = registry.content.create('ZotEndpoint')
        poco_service = registry.content.create('ZotPoco')

        self.add('site', site_service, registry=registry)
        self.add('hub', hub_service, registry=registry)
        self.add('channel', channel_service, registry=registry)
        self.add('xchannel', xchannel_service, registry=registry)
        self.add('message', message_service, registry=registry)
        self.add('post', endpoint_service, registry=registry)
        self.add('poco', poco_service, registry=registry)

    @property
    def site_url(self):
        try:
            return self._v_site_url
        except AttributeError:
            root = find_root(self)
            self._v_site_url = root.app_url
            return root.app_url

    @property
    def site_signature(self):
        try:
            return self._v_site_signature
        except AttributeError:
            signature = base64_url_encode(
                self._private_site_key.sign(self.site_url))
            self._v_site_signature = signature
            return signature

    @property
    def site_callback(self):
        try:
            return self._v_site_callback
        except AttributeError:
            url = '{0}{1}'.format(self.site_url,
                                  resource_path(self['post']))
            self._v_site_callback = url
            return url

    @property
    def site_callback_signature(self):
        try:
            return self._v_site_callback_signature
        except AttributeError:
            signature = base64_url_encode(
                self._private_site_key.sign(self.site_callback))
            self._v_site_callback_signature = signature
            return signature

    @property
    def public_site_key(self):
        return self._public_site_key

    def aes_decapsulate(self, data):
        return self._private_site_key.aes_decapsulate(data)

    def aes_decapsulate_json(self, data):
        result = {}
        if 'iv' in data:
            try:
                result = self.aes_decapsulate(data)
            except KeyError as e:
                # Data might not contain mandatory keys 'data' or 'key'
                log.error('aes_decapsulate_json: Missing keys in data.')
                log.exception(e)
                raise
            except TypeError as e:
                # Either the private key is None or some other
                # TypeError occured during decryption.
                log.error('aes_decapsulate_json: TypeError occured.')
                log.exception(e)
                raise
            except ValueError as e:
                log.error('aes_decapsulate_json: Could not decrypt received data.')
                log.exception(e)
                raise
            else:
                try:
                    result = json.loads(result.decode('utf-8'), encoding='utf-8')
                except ValueError as e:
                    log.error('aes_decapsulate_json: No valid JSON data received.')
                    log.exception(e)
                    raise
            return result
        # Data is not AES encapsulated
        return data

    def add_channel(self, nickname, name, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()

        prv_key = PersistentRSAKey()
        prv_key.generate_keypair()
        pub_key = prv_key.get_public_key()

        guid = self._create_channel_guid(nickname)
        signature = self._create_channel_signature(guid, prv_key)
        channel_hash = self.create_channel_hash(guid, signature)

        xchannel = registry.content.create('ZotLocalXChannel',
                                           nickname=nickname,
                                           name=name,  # TODO move to graph?
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
                                          name=name,  # TODO move to graph?
                                          channel_hash=channel_hash,
                                          guid=guid,  # TODO redundant with xchannel?
                                          signature=signature,  # TODO redundant with xchannel?
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
        return channel

    def _create_channel_guid(self, nickname, root=None):
        if root is None:
            root = find_root(self)
        uid = '{0}/{1}.{2}'.format(root.app_url, nickname,
                                   random.randint(0, sys.maxsize)
                                   ).encode('utf-8')
        wp = whirlpool.new(uid)
        return base64_url_encode(wp.digest())

    def _create_channel_signature(self, guid, key):
        return base64_url_encode(key.sign(guid))

    def create_channel_hash(self, guid, signature):
        """Create base64 encoded channel hash.

        Arguments 'guid' and 'signature' must be base64 encoded.
        """
        message = '{0}{1}'.format(guid, signature).encode('ascii')
        wp = whirlpool.new(message)
        return base64_url_encode(wp.digest())

    def import_xchannel(self, info, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()

        channel_hash = self.create_channel_hash(info['guid'],
                                                info['guid_sig'])
        log.debug('import xchan: channel_hash = {}'.format(channel_hash))

        pub_key = PersistentRSAKey(extern_public_key=info['key'])
        if not pub_key.verify(info['guid'],
                              base64_url_decode(info['guid_sig'])):
            log.error('import_xchannel: Unable to verify xchannel signature '
                      'for xchannel with hash {}.'.format(channel_hash))
            raise ValueError('Unable to verify channel signature')

        if '/' in info['address']:
            parts = info['address'].split(sep='/')
            info['address'] = parts[0]

        if '@' in info['address']:
            parts = info['address'].split(sep='@')
            nickname = parts[0]
        else:
            nickname = info['address']

        xchannel_service = self['xchannel']
        try:
            xchannel = xchannel_service[channel_hash]
        except KeyError:
            xchannel = registry.content.create(
                'ZotRemoteXChannel',
                nickname=nickname,
                name=info['name'],
                channel_hash=channel_hash,
                guid=info['guid'],
                signature=info['guid_sig'],
                key=pub_key,
                address=info['address'],
                url=info['url'],
                connections_url=info['connections_url'],
                photo=None,
                flags=None,
                *arg, **kw)
            xchannel_service.add(channel_hash, xchannel)
        else:
            xchannel.update(info)

        for location in info.get('locations', []):
            try:
                self.import_hub(info, location, channel_hash=channel_hash,
                                registry=registry, pub_key=pub_key)
            except ValueError:
                continue

        # TODO import photos and profiles

        try:
            self.import_site(info, info['site'],
                             registry=registry, pub_key=pub_key)
        except (KeyError, ValueError):
            pass

    def import_hub(self, info, location, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()

        channel_hash = kw.pop('channel_hash', None)
        if channel_hash is None:
            channel_hash = self.create_channel_hash(info['guid'],
                                                    info['guid_sig'])

        pub_key = kw.pop('pub_key', None)
        if pub_key is None:
            pub_key = PersistentRSAKey(extern_public_key=info['key'])
        if not pub_key.verify(location['url'],
                              base64_url_decode(location['url_sig'])):
            log.error('import_hub: Unable to verify hub signature '
                      'for hub with hash {}.'.format(channel_hash))
            raise ValueError('Unable to verify site signature')

        # TODO Allow channel clones
        if not location['primary']:
            return

        try:
            site_key = PersistentRSAKey(extern_public_key=location['sitekey'])
        except KeyError:
            raise ValueError('Empty hub site key')

        if '/' in location['address']:
            parts = info['address'].split(sep='/')
            location['address'] = parts[0]

        hub_service = self['hub']
        try:
            hub = hub_service[channel_hash]
        except KeyError:
            hub = registry.content.create(
                'ZotRemoteHub',
                channel_hash=channel_hash,
                guid=info['guid'],
                signature=info['guid_sig'],
                key=site_key,
                host=location['host'],
                address=location['address'],
                url=location['url'],
                url_signature=location['url_sig'],
                callback=location['callback'],
                *arg, **kw)
            hub_service.add(channel_hash, hub)
        else:
            hub.update(location)

    def import_site(self, info, site, *arg, **kw):
        registry = kw.pop('registry', None)
        if registry is None:
            registry = get_current_registry()

        url = site['url']

        pub_key = kw.pop('pub_key', None)
        if pub_key is None:
            pub_key = PersistentRSAKey(extern_public_key=info['key'])
        if not pub_key.verify(url,
                              base64_url_decode(site['url_sig'])):
            log.error('import_site: Unable to verify site signature '
                      'for site {}.'.format(url))
            raise ValueError('Unable to verify site signature')

        site_service = self['site']
        try:
            zot_site = site_service[url]
        except KeyError:
            zot_site = registry.content.create(
                'ZotSite',
                url=url,
                register_policy=site['register_policy'],
                access_policy=site['access_policy'],
                directory_mode=site['directory_mode'],
                directory_url=site['directory_url'],
                version=site['version'],
                admin_email=site['admin'],
                *arg, **kw)
            site_service.add(url, zot_site)
        else:
            zot_site.update(site)

    def import_messages(self, data, hub, **kw):
        report = []
        if 'iv' in data:
            try:
                data = self.aes_decapsulate_json(data)
            except (KeyError, TypeError, ValueError):
                log.error('import_messages: Rejected invalid AES '
                          'encapsulated pickup response.')
                raise ValueError('Invalid AES encapsulated pickup response')
        log.debug('import_messages: data = {}'.format(pformat(data)))
        try:
            incoming = data['pickup']
        except KeyError:
            log.error('import_messages: No pickup data available in response.')
            raise ValueError('No pickup data available in response')

        channel_service = self['channel']
        xchannel_service = self['xchannel']
        for item in incoming:
            try:
                notify = item['notify']
            except KeyError:
                log.error('import_messages: Invalid incoming message.')
                continue

            if 'iv' in notify:
                try:
                    notify = self.aes_decapsulate_json(notify)
                except (KeyError, TypeError, ValueError):
                    log.error('import_messages: Rejected invalid AES '
                              'encapsulated message.')
                    continue

            try:
                notify_sender = notify['sender']
            except KeyError:
                log.error('import_messages: No sender for incoming message '
                          'with secret {}.'.format(notify['secret']))
                continue
            if notify_sender['url'] != hub.url:
                log.error('import_messages: Potential forgery, '
                          'site {} is delivering as a sender with '
                          'guid {} from hub {}.'.format(notify_sender['url'],
                                                        notify_sender['guid'],
                                                        hub.url))
                continue
            sender_hash = self.create_channel_hash(notify_sender['guid'],
                                                   notify_sender['guid_sig'])
            sender = xchannel_service[sender_hash]

            try:
                message = item['message']
            except KeyError:
                log.error('import_messages: No message received in notify '
                          'with secret {} from channel {}.'.format(
                              notify['secret'], sender.channel_hash))
                continue

            try:
                message_type = message['type']
            except KeyError:
                log.error('import_messages: No message type received in '
                          'message with secret {} from channel {}.'.format(
                              notify['secret'], sender.channel_hash))
                continue

            try:
                recipients = notify['recipients']
            except KeyError:
                try:
                    if 'private' in message['flags']:
                        log.error('import_messages: Rejected private '
                                  'message without any recipients from '
                                  'channel {}.'.format(sender.channel_hash))
                        continue
                except KeyError:
                    pass
                log.info('import_messages: Received public message from '
                         'channel {}.'.format(sender.channel_hash))
                # TODO Check which local channels allow public messages
                filter_deliveries = (chan for chan in
                                     channel_service.values())  # if (
                                         # chan.allow_public))
            else:
                recipient_hashes = [self.create_channel_hash(
                    r['guid'], r['guid_sig']) for r in recipients]
                log.debug('import_messages: recipient_hashes = {}'.format(
                    recipient_hashes))
                filter_deliveries = (chan
                                     for chan in channel_service.values()
                                     for r_hash in recipient_hashes
                                     if chan.channel_hash == r_hash)

            try:
                registry = kw['request'].registry
            except KeyError:
                registry = get_current_registry()

            deliver_utility = 'deliver_{}'.format(message_type)
            try:
                DeliverUtility = registry.getUtility(IDeliverMessage,
                                                     deliver_utility)
            except ComponentLookupError:
                log.error('import_messages: No delivery method found '
                          'for message type {} from channel {}.'.format(
                              message_type, sender.channel_hash))
                continue
            message_dispatch = DeliverUtility()
            try:
                result = message_dispatch.deliver(sender, message,
                                                  filter_deliveries, **kw)
            except ValueError:
                continue
            report = report + result
        log.debug('import_messages: Delivery report '
                  '{}'.format(pformat(report)))
        return report

    def finger(self, address=None, channel_hash=None, site_url=None,
               target=None):
        """Finger a local or remote channel.

        ``address``, if passed, can be a local channel nickname or a remote
        channel address. Remote addresses are in the form ``nickname@host``
        or ``nickname@host:port``.

        When ``channel_hash`` is passed, finger request is sent to site
        ``site_url``.

        ``target`` is optional when querying addresses and represents the
        requesting channel.
        """
        result = {'success': False}
        scheme = 'https'
        well_known = '/.well-known/zot-info'
        query = {}
        payload = {}
        request_method = network.post
        if address is None and channel_hash is None:
            message = 'No channel address or hash supplied.'
            log.error('finger: {}'.format(message))
            raise ValueError(message)

        if channel_hash is not None:
            if site_url is None:
                message = 'No site URL specified.'
                log.error('finger: {}'.format(message))
                raise ValueError(message)
            else:
                url_parts = urlparse(site_url)
                scheme = url_parts.scheme
                netloc = url_parts.netloc
                query['guid_hash'] = channel_hash
                request_method = network.get
                if not netloc:
                    message = 'Invalid site URL {}.'.format(site_url)
                    log.error('finger: {}'.format(message))
                    raise ValueError(message)
        else:
            if '@' in address:
                parts = address.split(sep='@')
                nickname = parts[0]
                netloc = parts[1]
            else:
                root = find_root(self)
                nickname = address
                netloc = root.netloc
            if not nickname or not netloc:
                message = 'Invalid address {}.'.format(address)
                log.error('finger: {}'.format(message))
                raise ValueError(message)

            xchannel_address = '@'.join([nickname, netloc])
            log.debug('finger: Query for address '
                      '{}.'.format(xchannel_address))

            xchannel_service = self['xchannel']
            hub_service = self['hub']
            filter_xchan = (xchan for xchan in xchannel_service.values() if (
                xchan.address == xchannel_address))
            for xchannel in filter_xchan:
                try:
                    # Every xchannel should have a hub, but just in
                    # case something went wrong, no need to error out.
                    # We're just checking if one exists.
                    hub = hub_service[xchannel.channel_hash]
                except KeyError:
                    break
                url_parts = urlparse(hub.url)
                scheme = url_parts.scheme
                netloc = url_parts.netloc
                log.debug('finger: Found known hub '
                          'with URL {}.'.format(hub.url))
                break

            if IZotChannel.providedBy(target):
                payload = {'address': nickname,
                           'target': target.guid,
                           'target_sig': target.signature,
                           'key': target.key.export_public_key()}
                log.debug('finger: Payload {}.'.format(pformat(payload)))
            else:
                query['address'] = nickname
                request_method = network.get

        # Assemble the URL parts again.
        url = urlunparse((scheme, netloc, well_known, '', '', '',))
        log.debug('finger: Destination URL {}.'.format(url))
        try:
            response = request_method(url, params=query, data=payload)
        except (network.RequestException,
                network.ConnectionError) as e:
            log.error('finger: Caught outer network exception '
                      '{}.'.format(str(e)))
            if scheme.lower() == 'http':
                raise
            else:
                # Fall back to HTTP and assemble the URL parts again.
                scheme = 'http'
                url = urlunparse((scheme, netloc, well_known, '', '', '',))
                try:
                    response = request_method(url, params=query, data=payload)
                except (network.RequestException,
                        network.ConnectionError) as f:
                    log.error('finger: Caught inner network exception '
                              '{}.'.format(str(f)))
                    raise

        result = response.json()
        log.debug('finger: Result {}.'.format(pformat(result)))
        if not result['success']:
            if 'message' in result:
                log.error('finger: Response message is '
                          '{}.'.format(result['message']))
                raise ValueError(result['message'])
            else:
                message = 'No results.'
                log.error('finger: {}.'.format(message))
                raise ValueError(message)
        return result

    def fetch(self, data, hub, **kw):
        secret = data['secret']
        secret_signature = base64_url_encode(
            self._private_site_key.sign(secret))
        pickup = {'type': 'pickup',
                  'url': self.site_url,
                  'callback': self.site_callback,
                  'callback_sig': self.site_callback_signature,
                  'secret': secret,
                  'secret_sig': secret_signature}
        log.debug('fetch: Sending pickup for secret {} to endpoint {}.'.format(
            secret, hub.callback))
        pickup_data = json.dumps(pickup).encode('utf-8')
        pickup_data = json.dumps(hub.key.aes_encapsulate(pickup_data))
        try:
            result = self.zot(hub.callback, pickup_data)
        except (network.RequestException,
                network.ConnectionError) as e:
            log.error('fetch: Caught network exception %s' % (
                      str(e)))
            raise
        return self.import_messages(result, hub, **kw)

    def zot(self, url, data):
        data = {'data': data}
        response = network.post(url, data=data)
        log.debug('zot: Response status code {} '
                  'from url {}.'.format(response.status_code, url))
        return response.json()
