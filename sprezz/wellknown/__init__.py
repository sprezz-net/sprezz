import logging
import pprint

from persistent import Persistent
from pyramid.view import view_config

from ..content import content, service
from ..folder import Folder
from ..util.base64 import base64_url_decode
from ..util.crypto import PersistentRSAKey
from ..util.folder import find_service


log = logging.getLogger(__name__)


@service('WellKnown', service_name='well-known', after_create='after_create')
class WellKnown(Folder):
    def after_create(self, inst, registry):
        zot_info = registry.content.create('ZotInfo')
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
                 renderer='json')
    def zot_info(self):
        """Return .well-known/zot-info queries.

        This function answers zot-info queries and returns them conform Red
        API.

        The (post or get) request understands the following parameters:
        `address` is the nickname of the channel. 
        `guid` and `guid_sig` are the global ID and signature of the channel.
        `guid_hash` corresponds to the channel hash.
        `target` is the channel guid of the requestor, along with its
        `target_sig` signature and public `key`.
        """
        result = {'success': False}
        log.debug('params = %s' % pprint.pformat(self.request.params))

        zaddress = self.request.params.get('address', None)
        zguid = self.request.params.get('guid', None)
        zguid_sig = self.request.params.get('guid_sig', None)
        zhash = self.request.params.get('guid_hash', None)
        ztarget = self.request.params.get('target', '')
        ztarget_sig = self.request.params.get('target_sig', None)
        zkey = self.request.params.get('key', None)

        if ztarget is not '':
            if (zkey is None) or (ztarget_sig is None):
                result['message'] = 'No key or target signature supplied.'
                return result
            key = PersistentRSAKey(extern_public_key=zkey)
            if not key.verify_message(ztarget, base64_url_decode(ztarget_sig)):
                result['message'] = 'Invalid target signature.'
                return result

        zot_service = find_service(self.context, 'zot')
        channel_service = zot_service['channel']
        xchannel_service = zot_service['xchannel']

        xchannel = None
        try:
            # First try if zhash exists
            xchannel = xchannel_service[zhash]
        except KeyError:
            result['message'] = 'Item not found.'
            return result
        except TypeError:
            try:
                # Otherwise try if zaddress is a known channel
                zhash = channel_service[zaddress].channel_hash
                xchannel = xchannel_service[zhash]
            except KeyError:
                result['message'] = 'Item not found.'
                return result
            except TypeError:
                if (zguid is not None) and (zguid_sig is not None):
                    # Otherwise loop through channel to check for guid
                    for xchannel in xchannel_service:
                        if (xchannel.guid == zguid) and (
                                xchannel.signature == zguid_sig):
                            zhash = xchannel.channel_hash
                            xchannel = xchannel_service[zhash]
                            break
                    if xchannel is None:
                        result['message'] = 'Item not found.'
                        return result
                else:
                    result['message'] = 'Invalid request.'
                    return result

        # TODO Create a profile service which checks permissions
        profile = {
                'description': '',
                'birthday': '0000-00-00',
                'gender' : '',
                'marital' : '',
                'sexual' : '',
                'locale' : '',
                'region' : '',
                'postcode' : '',
                'keywords' : {},
                }
        result['profile'] = profile

        result['guid'] = xchannel.guid
        result['guid_sig'] = xchannel.signature
        result['key'] = xchannel.key.export_public_key()
        result['name'] = xchannel.name
        result['name_updated'] = '0000-00-00 00:00:00'
        result['address'] = xchannel.address
        result['photo_mimetype'] = ''
        result['photo'] = ''
        result['photo_updated'] = '0000-00-00 00:00:00'
        result['url'] = xchannel.url
        result['connections_url'] = xchannel.connections_url
        result['target'] = ztarget
        result['target_sig'] = ztarget_sig
        result['searchable'] = False

        # TODO permissions and hub locations
        hub_service = find_service(self.context, 'zot', 'hub')
        hub = hub_service[zhash]
        locations = []
        locations.append({
                'host': hub.host,
                'address' : hub.address,
                'primary' : True,
                'url' : hub.url,
                'url_sig' : hub.url_signature,
                'callback' : hub.callback,
                'sitekey': hub.key.export_public_key(),
                })
        result['locations'] = locations

        # TODO Site mode and policies
        site = {
                'url' : zot_service.site_url,
                'url_sig' : zot_service.site_signature,
                'directory_mode' : 0x0100,
                'directory_url' : '',
                'register_policy' : 0,
                'access_policy' : 0,
                'version' : 'Sprezz Matrix 0.1',
                'admin' : self.request.registry.settings['sprezz.admin.email'],
                }
        result['site'] = site

        result['success'] = True
        log.debug('result = %s' % pprint.pformat(result))
        return result

    @view_config(context=ZotChannelInfo,
                 request_method='POST',
                 renderer='json')
    def zot_channel_info(self):
        # TODO When is this triggered?
        result = {'success': False}
        log.debug('context = %s' % pprint.pformat(self.context))
        log.debug('graph = %s' % pprint.pformat(self.graph))
        log.debug('params = %s' % pprint.pformat(self.request.params))
        result['message'] = 'Not yet implemented'
        return result
