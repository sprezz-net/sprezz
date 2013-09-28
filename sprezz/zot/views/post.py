from zope.interface import implementer

from sprezz.interfaces import IPostEndpoint
from sprezz.util.folder import find_service


@implementer(IPostEndpoint)
class PostPing(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request

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
        return result


def includeme(config):
    config.registry.registerUtility(PostPing, IPostEndpoint, name='post_ping')
    config.hook_zca()
