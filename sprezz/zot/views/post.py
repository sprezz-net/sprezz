from zope.interface import implementer

from sprezz.interfaces import IPostEndpoint
from sprezz.util.base64 import base64_url_decode
from sprezz.util.folder import find_service


class AbstractPost(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request


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
        log.debug('post_ping: Received ping from {} at site {}.'.format(
            data['guid'], data['url']))
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


def includeme(config):
    config.registry.registerUtility(PostPing, IPostEndpoint,
                                    name='post_ping')
    config.registry.registerUtility(PostPickup, IPostEndpoint,
                                    name='post_pickup')
    config.hook_zca()
