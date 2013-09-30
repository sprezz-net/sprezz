import logging

from zope.interface import implementer

from sprezz.interfaces import IPostEndpoint
from sprezz.util.base64 import base64_url_decode
from sprezz.util.folder import find_service


log = logging.getLogger(__name__)


class AbstractPost(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request

    def get_primary_hub(self, sender):
        hub_service = find_service(self.context, 'zot', 'hub')
        filter_hub = (hub for hub in hub_service.values() if all([
            hub.url == sender['url'],
            hub.url_signature == sender['url_sig'],
            hub.guid == sender['guid'],
            hub.signature == sender['guid_sig']]))
        for hub in filter_hub:
            return hub
        raise ValueError('No primary hub found')

    def verify_sender(self, sender):
        try:
            hub = self.get_primary_hub(sender)
        except ValueError:
            # No primary hub found, register one
            zot_service = find_service(self.context, 'zot')
            channel_hash = zot_service.create_channel_hash(sender['guid'],
                                                           sender['guid_sig'])
            # FIXME try except around zot_finger
            info = zot_service.zot_finger(channel_hash=channel_hash,
                                          site_url=sender['url'])
            zot_service.import_xchannel(info)
            hub = self.get_primary_hub(sender)
        return hub


@implementer(IPostEndpoint)
class PostPing(AbstractPost):
    def post(self, data):
        zot_service = find_service(self.context, 'zot')
        result = {
            'success': True,
            'site': {
                'url': zot_service.site_url,
                'url_sig': zot_service.site_signature,
                'sitekey': zot_service.public_site_key.export_public_key()
                }
            }
        # Sender verification not required for ping, use it when available
        try:
            log.debug('post_ping: Received ping from '
                      'channel {} at site {}.'.format(data['sender']['guid'],
                                                      data['sender']['url']))
        except KeyError:
            log.debug('post_ping: Received ping.')
        return result


@implementer(IPostEndpoint)
class PostPickup(AbstractPost):
    def post(self, data):
        result = {'success': False}
        hub_service = find_service(self.context, 'zot', 'hub')
        filter_hub = (hub for hub in hub_service.values() if (
            hub.url == data['url'] and hub.callback == data['callback']))
        cb_ok = False
        sig_ok = False
        site_key = None
        for hub in filter_hub:
            site_key = hub.key
            cb_ok = site_key.verify(data['callback'],
                                    base64_url_decode(data['callback_sig']))
            sig_ok = site_key.verify(data['secret'],
                                     base64_url_decode(data['secret_sig']))
            if cb_ok and sig_ok:
                break
        if site_key is None:
            log.error('post_pickup: Site not found.')
            result['message'] = 'Site not found.'
            return result
        if not cb_ok:
            log.error('post_pickup: Possible site forgery.')
            result['message'] = 'Invalid callback signature.'
            return result
        if not sig_ok:
            log.error('post_pickup: Invalid secret signature.')
            result['message'] = 'Invalid secret signature.'
            return result
        # TODO Check if secret exists in outgoing queue
        # and deliver those messages
        log.debug('post_pickup: Need to deliver messages '
                  'with secret {}.'.format(data['secret']))
        site_key.aes_encapsulate(result)
        return result


@implementer(IPostEndpoint)
class PostNotify(AbstractPost):
    def post(self, data):
        result = {'success': False}
        zot_service = find_service(self.context, 'zot')
        try:
            sender = data['sender']
        except KeyError:
            log.error('post_notify: No sender.')
            result['message'] = 'No sender.'
            return result
        hub = self.verify_sender(sender)
        # TODO update hub with current date to show when we last communicated
        # successfully with this hub
        # TODO add ability for asynchronous fetch using a queue
        result['delivery_report'] = zot_service.zot_fetch(data, hub)
        result['success'] = True
        return result


def includeme(config):
    config.registry.registerUtility(PostPing, IPostEndpoint,
                                    name='post_ping')
    config.registry.registerUtility(PostPickup, IPostEndpoint,
                                    name='post_pickup')
    config.registry.registerUtility(PostNotify, IPostEndpoint,
                                    name='post_notify')
    config.hook_zca()
