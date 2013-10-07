import logging

from pyramid.view import view_config
from zope.component import ComponentLookupError

from ..channel import ZotChannels, ZotLocalChannel
from sprezz.interfaces import IDeliverMessage
from sprezz.util.folder import find_service


log = logging.getLogger(__name__)


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
        channel = zot_service.add_channel(nickname, name)
        return channel.__json__(self.request)

    @view_config(context=ZotChannels,
                 request_method='GET',
                 renderer='json')
    def list_channels(self):
        return {'channels': [c.__json__(self.request) for c in
                             self.context.values()]}

    @view_config(context=ZotLocalChannel,
                 request_method='GET',
                 name='matrix',
                 renderer='json')
    def list_matrix_messages(self):
        # TODO Filter messages based on owner
        message_service = find_service(self.context, 'zot', 'message')
        return {'messages': [m.__json__(self.request) for m in
                             message_service.values()]}

    @view_config(context=ZotLocalChannel,
                 request_method='POST',
                 name='matrix',
                 renderer='json')
    def post_message(self):
        zot_service = find_service(self.context, 'zot')
        hub_service = zot_service['hub']
        hub = hub_service[self.context.channel_hash]

        params = self.request.params
        message = {
            'type': 'activity',
            'title': params.get('title', '').strip(),
            'body': params.get('body', ''),
            'mimetype': params.get('mimetype', 'text/bbcode'),
            'verb': 'http://activitystrea.ms/schema/1.0/post',
            'app': params.get('app', 'Sprezz'),
            'author': {
                'name': self.context.name,
                'address': self.context.address,
                'url': hub.url,
                'guid': self.context.guid,
                'guid_sig': self.context.signature,
                },
            'owner': {
                'name': self.context.name,
                'address': self.context.address,
                'url': hub.url,
                'guid': self.context.guid,
                'guid_sig': self.context.signature,
                },
            }
        # outq_service = zot_service['outqueue']
        channel_service = zot_service['channel']
        xchannel_service = zot_service['xchannel']
        registry = self.request.registry
        try:
            DeliverUtility = registry.getUtility(IDeliverMessage,
                                                 'deliver_activity')
        except ComponentLookupError:
            log.error('post_message: No delivery method found.')
            return
        message_dispatch = DeliverUtility()
        sender = xchannel_service[self.context.channel_hash]
        recipients = [chan for chan in channel_service.values()]
        try:
            message_dispatch.deliver(sender, message, recipients,
                                     registry=registry)
        except (ValueError, KeyError) as e:
            # KeyError occurs when no available message_id was found.
            # ValueError occurs for possible forgery, which should not
            # happen because the local channel is the author.
            log.exception(e)
            return
        message_service = zot_service['message']
        item = message_service[message['message_id']]
        log.debug('post_message: Created message with id '
                  '{}.'.format(item.message_id))
        return item

    @view_config(context=ZotLocalChannel,
                 request_method='POST',
                 name='connection',
                 renderer='json')
    def add_connection(self):
        result = {'success': False}
        url = self.request.params['url'].strip()
        zot_service = find_service(self.context, 'zot')
        #try:
        info = zot_service.finger(address=url, target=self.context)
        #except Exception as e:
            # TODO Check for the various exception classes like HTTP
            # exeception, etc.
            #result['message'] = str(e)
            #return result

        zot_service.import_xchannel(info)

        result['success'] = True
        result['info'] = info
        return result
