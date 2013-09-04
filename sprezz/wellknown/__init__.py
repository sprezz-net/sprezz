import logging
import pprint

from persistent import Persistent
from pyramid.view import view_config

from ..content import content, service
from ..crypto import PersistentRSAKey
from ..folder import Folder
from ..util import base64_url_decode


log = logging.getLogger(__name__)


@service('WellKnown', service_name='well-known', after_create='after_create')
class WellKnown(Folder):
    def after_create(self, inst, registry):
        zot_info = ZotInfo()
        self.add('zot-info', zot_info)


@content('ZotInfo')
class ZotInfo(Persistent):
    def __getitem__(self, key):
        # TODO Check if address key exists
        if True:
            dispatch = ZotChannelInfo()
            dispatch.__parent__ = self
            dispatch.__name__ = key
            return dispatch
        raise KeyError(key)


class ZotChannelInfo(object):
    pass


class ZotInfoProtocol(object):
    def __init__(self, context, request):
        self.context = context
        self.request = request
        self.graph = request.graph

    @view_config(context=ZotInfo,
                 request_method='POST',
                 renderer='json')
    def zot_info(self):
        result = {'success': False}
        log.debug('params = %s' % pprint.pformat(self.request.params))

        zaddress = self.request.params.get('address', None)
        ztarget = self.request.params.get('target', '')
        ztarget_sig = self.request.params.get('target_sig', None)
        zkey = self.request.params.get('key', None)
        if (zkey is None) or (ztarget_sig is None):
            result['message'] = 'zfinger: No key or target signature supplied'
        key = PersistentRSAKey(extern_public_key=zkey)
        if not key.verify_message(ztarget, base64_url_decode(ztarget_sig)):
            result['message'] = 'zfinger: Invalid target signature'

        # TODO Check if zaddress is known here and return profile info

        #result['success'] = True
        log.debug('result = %s' % pprint.pformat(result))
        return result

    @view_config(context=ZotChannelInfo,
                 request_method='POST',
                 renderer='json')
    def zot_channel_info(self):
        result = {'success': False}
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        log.debug('params = %s' % pprint.pformat(self.request.params))
        result['message'] = 'Not yet implemented'
        return result
